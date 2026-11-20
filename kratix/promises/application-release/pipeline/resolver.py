from __future__ import annotations

import json
import os
import re
import ssl
import sys
import urllib.error
import urllib.request
from dataclasses import dataclass
from pathlib import Path
from typing import Any

import yaml


INPUT_DIR = Path(os.getenv("KRATIX_INPUT_DIR", "/kratix/input"))
OUTPUT_DIR = Path(os.getenv("KRATIX_OUTPUT_DIR", "/kratix/output"))
METADATA_DIR = Path(os.getenv("KRATIX_METADATA_DIR", "/kratix/metadata"))


PROFILE_API_VERSION = "runtimeconditions.io/v1alpha1"
PROFILE_KIND = "RuntimeConditionsProfile"

API_PROPERTIES = {"baseUrl"}
REDIS_PROPERTIES = {"url", "hostname", "port"}
# In-cluster Redis is naturally addressed as a Service host and port.
REDIS_PREFERRED_ALTERNATIVES = [{"hostname", "port"}]


class ContractError(Exception):
    pass


class ProfileError(Exception):
    """spec.profile cannot be read as a Profile envelope.

    This is a defensive check, not Runtime Conditions validation; the profiler
    and extension schemas establish validity when the Profile is generated.
    """


@dataclass
class UnsupportedCondition:
    name: str
    kind: str
    reason: str


class UnsupportedConditionError(Exception):
    """Valid demand that this platform does not know how to fulfill."""

    def __init__(self, conditions: list[UnsupportedCondition]):
        self.conditions = conditions
        super().__init__(
            "\n".join(f"condition {c.name!r} (kind {c.kind!r}): {c.reason}" for c in conditions)
        )


@dataclass
class SupportedCondition:
    condition: dict[str, Any]
    binding: str
    env: list[tuple[str, str]]


class NoAliasDumper(yaml.SafeDumper):
    def ignore_aliases(self, data: Any) -> bool:
        return True


@dataclass
class CatalogAPI:
    name: str
    namespace: str
    definition: dict[str, Any]
    openapi: dict[str, Any]
    base_url: str | None


def main() -> int:
    try:
        request = yaml.safe_load((INPUT_DIR / "object.yaml").read_text(encoding="utf-8"))
        output = resolve(request)
        OUTPUT_DIR.mkdir(parents=True, exist_ok=True)
        write_yaml_documents(OUTPUT_DIR / "application-release.yaml", output)
        write_status(
            {
                "message": "Application release resolved",
                "resolvedConditions": {
                    "apis": output.summary["apis"],
                    "caches": output.summary["caches"],
                },
            }
        )
        return 0
    except ProfileError as exc:
        print(f"invalid Runtime Conditions Profile\n\n{exc}", file=sys.stderr)
        write_status({"message": "Invalid Runtime Conditions Profile", "invalidProfile": str(exc)})
        return 1
    except UnsupportedConditionError as exc:
        print(f"unsupported Runtime Conditions\n\n{exc}", file=sys.stderr)
        write_status(
            {
                "message": "Unsupported Runtime Conditions",
                "unsupportedConditions": [
                    {"name": c.name, "kind": c.kind, "reason": c.reason} for c in exc.conditions
                ],
            }
        )
        return 1
    except ContractError as exc:
        message = f"API contract validation failed\n\n{exc}"
        print(message, file=sys.stderr)
        write_status({"message": "API contract validation failed", "validationError": str(exc)})
        return 1
    except Exception as exc:
        print(f"application release resolution failed: {exc}", file=sys.stderr)
        write_status({"message": "Application release resolution failed", "error": str(exc)})
        return 1


class OutputDocuments(list):
    def __init__(self, values: list[dict[str, Any]], summary: dict[str, Any]):
        super().__init__(values)
        self.summary = summary


def resolve(request: dict[str, Any]) -> OutputDocuments:
    metadata = request.get("metadata", {})
    spec = request.get("spec", {})
    name = metadata["name"]
    namespace = metadata.get("namespace", "default")
    image = require_string(spec, "image")
    port = int(spec.get("port", 8080))
    replicas = int(spec.get("replicas", 1))
    image_pull_policy = spec.get("imagePullPolicy", "Always")
    readiness_path = spec.get("readinessPath", "/ready")

    conditions = read_profile_conditions(require_string(spec, "profile"))
    supported = evaluate_support(conditions)

    catalog = load_catalog(spec.get("catalog", {}).get("configMapRef"))
    apis = parse_catalog_apis(catalog)

    env: list[dict[str, str]] = []
    emitted: list[dict[str, Any]] = []
    summary: dict[str, Any] = {"apis": [], "caches": []}

    for item in supported:
        if item.binding != "catalog-api":
            continue
        condition = item.condition
        api = validate_api_condition(condition, apis)
        base_url = api.base_url or f"http://{api.name}.{namespace}.svc.cluster.local:{port}"
        bound = bind_env(item.env, {"baseUrl": base_url})
        env.extend(bound)
        summary["apis"].append(
            {
                "condition": condition.get("name"),
                "catalogApi": api.name,
                "env": [var["name"] for var in bound],
                "url": base_url,
            }
        )

    redis_cache_count = 0
    for item in supported:
        if item.binding != "redis":
            continue
        condition = item.condition
        redis_cache_count += 1
        redis_name = f"{name}-cache" if redis_cache_count == 1 else f"{name}-cache-{redis_cache_count}"
        emitted.append(redis_request(redis_name, namespace, name))
        redis_host = f"{redis_name}.{namespace}.svc.cluster.local"
        bound = bind_env(
            item.env,
            {"url": f"redis://{redis_host}:6379", "hostname": redis_host, "port": "6379"},
        )
        env.extend(bound)
        summary["caches"].append(
            {
                "condition": condition.get("name"),
                "resource": redis_name,
                "engine": "redis",
                "env": [var["name"] for var in bound],
            }
        )

    emitted.extend(
        [
            workload_deployment(name, namespace, image, image_pull_policy, port, replicas, readiness_path, env),
            workload_service(name, namespace, port),
        ]
    )
    return OutputDocuments(emitted, summary)


def read_profile_conditions(text: str) -> list[dict[str, Any]]:
    try:
        profile = yaml.safe_load(text)
    except yaml.YAMLError as exc:
        raise ProfileError(f"spec.profile is not YAML: {exc}") from exc
    if not isinstance(profile, dict):
        raise ProfileError("spec.profile must be a mapping")

    problems: list[str] = []
    if profile.get("apiVersion") != PROFILE_API_VERSION:
        problems.append(f"apiVersion is {profile.get('apiVersion')!r}, expected {PROFILE_API_VERSION!r}")
    if profile.get("kind") != PROFILE_KIND:
        problems.append(f"kind is {profile.get('kind')!r}, expected {PROFILE_KIND!r}")
    conditions = profile.get("conditions")
    if not isinstance(conditions, list):
        found = "missing" if "conditions" not in profile else type(conditions).__name__
        problems.append(f"conditions must be a list, got {found}")
        conditions = []

    seen: set[str] = set()
    for index, condition in enumerate(conditions):
        if isinstance(condition, dict) and "name" not in condition:
            condition["name"] = f"conditions[{index}]"
        if not isinstance(condition, dict) or not condition.get("name") or not condition.get("kind"):
            problems.append(f"conditions[{index}] must have a name and kind")
            continue
        if condition["name"] in seen:
            problems.append(f"conditions[{index}]: duplicate condition name {condition['name']!r}")
        seen.add(condition["name"])

    if problems:
        raise ProfileError("\n".join(problems))
    return conditions


def evaluate_support(conditions: list[dict[str, Any]]) -> list[SupportedCondition]:
    """Account for every Condition before fulfilling any of them."""
    supported: list[SupportedCondition] = []
    unsupported: list[UnsupportedCondition] = []
    for condition in conditions:
        binding, provided, preferred, reason = support_for(condition)
        env: list[tuple[str, str]] = []
        if reason is None:
            env, reason = select_env(condition, provided, preferred)
        if reason is None:
            supported.append(SupportedCondition(condition, binding, env))
        else:
            unsupported.append(UnsupportedCondition(condition["name"], condition["kind"], reason))
    if unsupported:
        raise UnsupportedConditionError(unsupported)
    return supported


def support_for(condition: dict[str, Any]) -> tuple[str, set[str], list[set[str]], str | None]:
    """This platform's support table: binding, provided properties, preferences, or a reason."""
    kind = condition["kind"]
    interface = condition.get("interface") or {}
    if kind == "api":
        if interface.get("type") != "http":
            return "", set(), [], f"no platform binding for API interface type {interface.get('type')!r}"
        operations = interface.get("operations") or []
        if operations:
            return "catalog-api", API_PROPERTIES, [], None
        spec = api_spec(condition)
        if spec.get("format", "openapi") != "openapi":
            return "", set(), [], f"no platform binding for API spec format {spec.get('format')!r}"
        if "uri" in spec and parse_catalog_ref(spec["uri"]) is None:
            return "", set(), [], f"no platform binding for API spec uri {spec.get('uri')!r}"
        if "version" in spec and parse_constraint(spec["version"]) is None:
            return "", set(), [], f"version requirement {spec['version']!r} is not a constraint this platform understands"
        return "catalog-api", API_PROPERTIES, [], None
    if kind == "cache":
        if interface.get("type") != "key_value" or interface.get("engine") != "redis":
            return (
                "",
                set(),
                [],
                f"no platform binding for cache type {interface.get('type')!r} engine {interface.get('engine')!r}",
            )
        return "redis", REDIS_PROPERTIES, REDIS_PREFERRED_ALTERNATIVES, None
    return "", set(), [], f"no platform binding for condition kind {kind!r}"


def select_env(
    condition: dict[str, Any], provided: set[str], preferred: list[set[str]]
) -> tuple[list[tuple[str, str]], str | None]:
    """Choose the (env name, property) pairs to satisfy, or return why not.

    For alternatives, exactly one complete alternative is chosen: the first
    satisfiable one matching a platform preference, else the first satisfiable
    one in declared order.
    """
    name = condition["name"]
    configuration = condition.get("configuration")
    if not configuration:
        return [], "condition declares no configuration; this platform only supplies env configuration"
    if not isinstance(configuration, dict):
        raise ProfileError(f"condition {name!r}: configuration must be a mapping")
    unknown = sorted(set(configuration) - {"env", "alternatives"})
    if unknown:
        return [], f"unsupported configuration form {unknown[0]!r}"
    if len(configuration) > 1:
        return [], "configuration declares both env and alternatives; this platform supports one form per condition"

    if "env" in configuration:
        entries = env_entries(name, "configuration.env", configuration["env"])
        missing = [prop for _, prop in entries if prop not in provided]
        if missing:
            return [], f"platform binding does not provide property {missing[0]!r}"
        return entries, None

    alternatives = configuration["alternatives"]
    if not isinstance(alternatives, list):
        raise ProfileError(f"condition {name!r}: configuration.alternatives must be a list")
    satisfiable: list[list[tuple[str, str]]] = []
    for index, alternative in enumerate(alternatives):
        raw = alternative.get("env") if isinstance(alternative, dict) else None
        entries = env_entries(name, f"configuration.alternatives[{index}].env", raw)
        if all(prop in provided for _, prop in entries):
            satisfiable.append(entries)
    if not satisfiable:
        return [], f"platform binding provides {sorted(provided)} but no alternative is fully satisfiable"
    for preference in preferred:
        for entries in satisfiable:
            if {prop for _, prop in entries} == preference:
                return entries, None
    return satisfiable[0], None


def env_entries(condition_name: str, path: str, raw: Any) -> list[tuple[str, str]]:
    if not isinstance(raw, list) or not raw:
        raise ProfileError(f"condition {condition_name!r}: {path} must be a non-empty list")
    entries: list[tuple[str, str]] = []
    for index, entry in enumerate(raw):
        if not isinstance(entry, dict) or not entry.get("property") or not entry.get("name"):
            raise ProfileError(f"condition {condition_name!r}: {path}[{index}] must have property and name")
        entries.append((entry["name"], entry["property"]))
    return entries


def bind_env(entries: list[tuple[str, str]], values: dict[str, str]) -> list[dict[str, str]]:
    return [{"name": env_name, "value": values[prop]} for env_name, prop in entries]


def validate_api_condition(condition: dict[str, Any], apis: list[CatalogAPI]) -> CatalogAPI:
    interface = condition.get("interface") or {}
    if interface.get("type") != "http":
        raise ContractError(f"condition {condition.get('name')}: unsupported API interface {interface.get('type')!r}")

    operations = interface.get("operations") or []
    api = find_api(condition, apis, operations)
    for operation in operations:
        validate_operation(condition, operation, api)
    if not operations:
        validate_version(condition, api)
    return api


def api_spec(condition: dict[str, Any]) -> dict[str, Any]:
    spec = (condition.get("interface") or {}).get("spec") or {}
    if not isinstance(spec, dict):
        raise ProfileError(f"condition {condition['name']!r}: interface.spec must be a mapping")
    return spec


def parse_catalog_ref(uri: Any) -> tuple[str, str] | None:
    """catalog://api/<namespace>/<name> -> (namespace, name)."""
    match = re.fullmatch(r"catalog://api/([^/]+)/([^/]+)", uri) if isinstance(uri, str) else None
    return (match.group(1), match.group(2)) if match else None


def parse_constraint(value: Any) -> tuple[str, tuple[int, int, int]] | None:
    """Exact MAJOR.MINOR.PATCH or one of =, >, >=, <, <=, ^, ~ as in common-integrations."""
    match = re.fullmatch(r"(=|>=|<=|>|<|\^|~)?(\d+)\.(\d+)\.(\d+)", value) if isinstance(value, str) else None
    return (match.group(1) or "=", tuple(int(part) for part in match.group(2, 3, 4))) if match else None


def satisfies_version(constraint: str, version: str) -> bool:
    parsed = parse_constraint(constraint)
    if parsed is None:
        raise ValueError(f"version requirement {constraint!r} is not a constraint this platform understands")
    concrete = re.fullmatch(r"(\d+)\.(\d+)\.(\d+)", version) if isinstance(version, str) else None
    if concrete is None:
        raise ValueError(f"published version {version!r} is not MAJOR.MINOR.PATCH")
    operator, want = parsed
    have = tuple(int(part) for part in concrete.groups())
    if operator == "=":
        return have == want
    if operator == ">":
        return have > want
    if operator == ">=":
        return have >= want
    if operator == "<":
        return have < want
    if operator == "<=":
        return have <= want
    if operator == "^":
        return want <= have < (want[0] + 1, 0, 0)
    return want <= have < (want[0], want[1] + 1, 0)


def validate_version(condition: dict[str, Any], api: CatalogAPI) -> None:
    spec = api_spec(condition)
    if "version" not in spec:
        return
    declared = spec["version"]
    published = (api.openapi.get("info") or {}).get("version")
    try:
        satisfied = satisfies_version(declared, published)
    except ValueError as exc:
        raise ContractError(f"condition api: {condition.get('name')}\nresult: {exc}") from exc
    if not satisfied:
        raise ContractError(
            f"condition:\n  api: {condition.get('name')}\n\n"
            f"expected by workload:\n  version: {declared}\n\n"
            f"published by catalog:\n  version: {published}\n\n"
            "result:\n  incompatible"
        )


def api_satisfies_operations(
    condition: dict[str, Any], api: CatalogAPI, operations: list[dict[str, Any]]
) -> bool:
    try:
        for operation in operations:
            validate_operation(condition, operation, api)
    except ContractError:
        return False
    return True


def find_api(condition: dict[str, Any], apis: list[CatalogAPI], operations: list[dict[str, Any]]) -> CatalogAPI:
    condition_name = condition.get("name")
    spec = api_spec(condition)

    if operations:
        candidates: list[CatalogAPI] = []

        if "uri" in spec:
            reference = parse_catalog_ref(spec["uri"])
            if reference is not None:
                namespace, name = reference
                candidates.extend(api for api in apis if api.namespace == namespace and api.name == name)

        candidates.extend(api for api in apis if api.name == condition_name and api not in candidates)
        candidates.extend(api for api in apis if api not in candidates)

        # Prefer a provider that satisfies the complete explicit operation
        # contract, regardless of conflicting spec hints.
        for api in candidates:
            if api_satisfies_operations(condition, api, operations):
                return api

        # If no provider fully satisfies the schema, preserve the most relevant
        # method/path match so validate_operation can report the concrete
        # incompatibility instead of collapsing it into "no matching API".
        for api in candidates:
            if all(openapi_has_operation(api.openapi, op.get("method"), op.get("path")) for op in operations):
                return api

        raise ContractError(
            f"condition api: {condition_name}\nresult: no catalog API satisfies the declared operations"
        )

    if "uri" in spec:
        reference = parse_catalog_ref(spec["uri"])
        if reference is None:
            raise ContractError(
                f"condition api: {condition_name}\nspec uri: {spec['uri']}\nresult: unsupported catalog reference"
            )
        namespace, name = reference
        for api in apis:
            if api.namespace == namespace and api.name == name:
                return api
        raise ContractError(
            f"condition api: {condition_name}\nspec uri: {spec['uri']}\nresult: no catalog API with that reference"
        )

    for api in apis:
        if api.name == condition_name:
            return api

    raise ContractError(f"condition api: {condition_name}\nresult: no matching catalog API")


def validate_operation(condition: dict[str, Any], operation: dict[str, Any], api: CatalogAPI) -> None:
    method = str(operation.get("method", "")).lower()
    path = operation.get("path")
    operation_doc = ((api.openapi.get("paths") or {}).get(path) or {}).get(method)
    if operation_doc is None:
        raise ContractError(
            f"condition:\n  api: {condition.get('name')}\n  operation: {method.upper()} {path}\n\n"
            "result:\n  missing operation in published OpenAPI"
        )

    expected = operation.get("responseSchema")
    if not expected:
        return

    published = response_schema(api.openapi, operation_doc)
    if published is None:
        raise ContractError(
            f"condition:\n  api: {condition.get('name')}\n  operation: {method.upper()} {path}\n\n"
            "result:\n  missing JSON response schema in published OpenAPI"
        )
    compare_schema(condition.get("name"), method.upper(), path, expected, published, api.openapi, "response")


def compare_schema(
    api_name: str,
    method: str,
    path: str,
    expected: Any,
    published: Any,
    openapi: dict[str, Any],
    location: str,
) -> None:
    published = resolve_ref(openapi, published)
    if isinstance(expected, str):
        published_type = published.get("type") if isinstance(published, dict) else published
        if published_type != expected:
            raise ContractError(
                f"condition:\n  api: {api_name}\n  operation: {method} {path}\n\n"
                f"expected by workload:\n  {location}: {expected}\n\n"
                f"published by catalog:\n  {location}: {published_type}\n\n"
                "result:\n  incompatible"
            )
        return

    if isinstance(expected, list):
        if not isinstance(published, dict) or published.get("type") != "array":
            raise ContractError(
                f"condition:\n  api: {api_name}\n  operation: {method} {path}\n\n"
                f"expected by workload:\n  {location}: array\n\n"
                f"published by catalog:\n  {location}: {published.get('type') if isinstance(published, dict) else published}\n\n"
                "result:\n  incompatible"
            )
        if expected:
            compare_schema(api_name, method, path, expected[0], published.get("items", {}), openapi, f"{location}[]")
        return

    if isinstance(expected, dict):
        if isinstance(published, dict) and published.get("type") not in (None, "object"):
            raise ContractError(
                f"condition:\n  api: {api_name}\n  operation: {method} {path}\n\n"
                f"expected by workload:\n  {location}: object\n\n"
                f"published by catalog:\n  {location}: {published.get('type')}\n\n"
                "result:\n  incompatible"
            )
        properties = published.get("properties", {}) if isinstance(published, dict) else {}
        for field, field_expected in expected.items():
            if field not in properties:
                raise ContractError(
                    f"condition:\n  api: {api_name}\n  operation: {method} {path}\n\n"
                    f"expected by workload:\n  {location}.{field}: present\n\n"
                    f"published by catalog:\n  {location}.{field}: missing\n\n"
                    "result:\n  incompatible"
                )
            compare_schema(api_name, method, path, field_expected, properties[field], openapi, f"{location}.{field}")


def response_schema(openapi: dict[str, Any], operation_doc: dict[str, Any]) -> dict[str, Any] | None:
    responses = operation_doc.get("responses") or {}
    response = responses.get("200") or next((value for key, value in responses.items() if str(key).startswith("2")), None)
    if not response:
        return None
    response = resolve_ref(openapi, response)
    content = response.get("content") or {}
    media = content.get("application/json") or next(iter(content.values()), None)
    if not media:
        return None
    return resolve_ref(openapi, media.get("schema") or {})


def resolve_ref(document: dict[str, Any], value: Any) -> Any:
    if not isinstance(value, dict) or "$ref" not in value:
        return value
    ref = value["$ref"]
    if not ref.startswith("#/"):
        raise ContractError(f"unsupported external OpenAPI reference: {ref}")
    current: Any = document
    for part in ref[2:].split("/"):
        current = current[part.replace("~1", "/").replace("~0", "~")]
    return current


def openapi_has_operation(openapi: dict[str, Any], method: str | None, path: str | None) -> bool:
    if not method or not path:
        return False
    return str(method).lower() in ((openapi.get("paths") or {}).get(path) or {})


def load_catalog(config_map_ref: dict[str, Any] | None) -> dict[str, str]:
    local_dir = os.getenv("RUNTIME_CONDITIONS_CATALOG_DIR")
    if local_dir:
        return {path.name: path.read_text(encoding="utf-8") for path in Path(local_dir).iterdir() if path.is_file()}

    if not config_map_ref:
        raise ContractError("spec.catalog.configMapRef is required")
    namespace = config_map_ref["namespace"]
    name = config_map_ref["name"]

    host = os.getenv("KUBERNETES_SERVICE_HOST")
    port = os.getenv("KUBERNETES_SERVICE_PORT", "443")
    token_path = Path("/var/run/secrets/kubernetes.io/serviceaccount/token")
    ca_path = Path("/var/run/secrets/kubernetes.io/serviceaccount/ca.crt")
    if not host or not token_path.exists():
        raise ContractError("cannot load catalog ConfigMap outside Kubernetes without RUNTIME_CONDITIONS_CATALOG_DIR")

    token = token_path.read_text(encoding="utf-8")
    url = f"https://{host}:{port}/api/v1/namespaces/{namespace}/configmaps/{name}"
    request = urllib.request.Request(url, headers={"Authorization": f"Bearer {token}"})
    context = ssl.create_default_context(cafile=str(ca_path))
    try:
        with urllib.request.urlopen(request, context=context, timeout=10) as response:
            configmap = json.loads(response.read().decode("utf-8"))
    except urllib.error.HTTPError as exc:
        raise ContractError(f"could not read catalog ConfigMap {namespace}/{name}: HTTP {exc.code}") from exc
    return configmap.get("data") or {}


def parse_catalog_apis(catalog_data: dict[str, str]) -> list[CatalogAPI]:
    apis: list[CatalogAPI] = []
    for key, value in catalog_data.items():
        if not key.endswith((".catalog-info.yaml", ".catalog-info.yml")):
            continue
        for entity in yaml.safe_load_all(value):
            if not entity or entity.get("kind") != "API":
                continue
            name = entity.get("metadata", {}).get("name")
            definition_ref = (
                entity.get("spec", {})
                .get("definition", {})
                .get("$text")
            )
            if not definition_ref:
                continue
            definition_key = Path(str(definition_ref).removeprefix("./")).name
            if definition_key not in catalog_data:
                raise ContractError(f"catalog API {name} references missing OpenAPI document {definition_ref}")
            annotations = entity.get("metadata", {}).get("annotations", {})
            apis.append(
                CatalogAPI(
                    name=name,
                    namespace=entity.get("metadata", {}).get("namespace") or "default",
                    definition=entity,
                    openapi=yaml.safe_load(catalog_data[definition_key]),
                    base_url=annotations.get("platform.demoteam.io/base-url"),
                )
            )
    return apis


def redis_request(name: str, namespace: str, workload_name: str) -> dict[str, Any]:
    return {
        "apiVersion": "platform.demoteam.io/v1alpha1",
        "kind": "Redis",
        "metadata": {
            "name": name,
            "namespace": namespace,
            "labels": {
                "kratix.io/component-of-promise-name": "application-release",
                "kratix.io/component-of-resource-name": workload_name,
                "kratix.io/component-of-resource-namespace": namespace,
            },
        },
        "spec": {"size": "small"},
    }


def workload_deployment(
    name: str,
    namespace: str,
    image: str,
    image_pull_policy: str,
    port: int,
    replicas: int,
    readiness_path: str,
    env: list[dict[str, str]],
) -> dict[str, Any]:
    labels = {
        "app.kubernetes.io/name": name,
        "app.kubernetes.io/component": "application",
        "app.kubernetes.io/managed-by": "kratix",
    }
    return {
        "apiVersion": "apps/v1",
        "kind": "Deployment",
        "metadata": {"name": name, "namespace": namespace, "labels": labels},
        "spec": {
            "replicas": replicas,
            "selector": {"matchLabels": {"app.kubernetes.io/name": name}},
            "template": {
                "metadata": {"labels": labels},
                "spec": {
                    "containers": [
                        {
                            "name": "app",
                            "image": image,
                            "imagePullPolicy": image_pull_policy,
                            "ports": [{"name": "http", "containerPort": port}],
                            "env": env,
                            "readinessProbe": {
                                "httpGet": {"path": readiness_path, "port": "http"},
                                "initialDelaySeconds": 3,
                                "periodSeconds": 5,
                            },
                            "livenessProbe": {
                                "httpGet": {"path": readiness_path, "port": "http"},
                                "initialDelaySeconds": 15,
                                "periodSeconds": 10,
                            },
                        }
                    ]
                },
            },
        },
    }


def workload_service(name: str, namespace: str, port: int) -> dict[str, Any]:
    return {
        "apiVersion": "v1",
        "kind": "Service",
        "metadata": {
            "name": name,
            "namespace": namespace,
            "labels": {
                "app.kubernetes.io/name": name,
                "app.kubernetes.io/managed-by": "kratix",
            },
        },
        "spec": {
            "selector": {"app.kubernetes.io/name": name},
            "ports": [{"name": "http", "port": port, "targetPort": "http"}],
        },
    }


def require_string(mapping: dict[str, Any], key: str) -> str:
    value = mapping.get(key)
    if not isinstance(value, str) or not value:
        raise ContractError(f"spec.{key} is required")
    return value


def write_yaml_documents(path: Path, documents: list[dict[str, Any]]) -> None:
    with path.open("w", encoding="utf-8") as file:
        yaml.dump_all(documents, file, Dumper=NoAliasDumper, sort_keys=False)


def write_status(status: dict[str, Any]) -> None:
    METADATA_DIR.mkdir(parents=True, exist_ok=True)
    (METADATA_DIR / "status.yaml").write_text(
        yaml.dump(status, Dumper=NoAliasDumper, sort_keys=False),
        encoding="utf-8",
    )


if __name__ == "__main__":
    raise SystemExit(main())
