import os
import subprocess
import tempfile
import unittest
from pathlib import Path

import yaml

import resolver

REPO_ROOT = Path(__file__).resolve().parents[4]
PROFILE = REPO_ROOT / "artifacts" / "request-logger-http.profile.yaml"
MATERIALIZE = REPO_ROOT / "kratix" / "scripts" / "materialize-application-release.sh"
CATALOG = REPO_ROOT / "kratix" / "manifests" / "catalog" / "todos-api-catalog.yaml"
BREAKING_CATALOG = REPO_ROOT / "kratix" / "manifests" / "catalog" / "todos-api-catalog-breaking.yaml"


class ResolverTestCase(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.use_catalog(CATALOG)

    def use_catalog(self, configmap_path):
        catalog_dir = Path(self.tmp.name) / f"catalog-{configmap_path.stem}"
        catalog_dir.mkdir(exist_ok=True)
        for key, value in yaml.safe_load(configmap_path.read_text())["data"].items():
            (catalog_dir / key).write_text(value)
        os.environ["RUNTIME_CONDITIONS_CATALOG_DIR"] = str(catalog_dir)
        self.addCleanup(os.environ.pop, "RUNTIME_CONDITIONS_CATALOG_DIR", None)

    def materialized_release(self, name="request-logger"):
        output = Path(self.tmp.name) / "release.yaml"
        subprocess.run(
            [str(MATERIALIZE), name, str(PROFILE), str(output)],
            check=True,
            capture_output=True,
        )
        return yaml.safe_load(output.read_text())

    def release_with_conditions(self, conditions):
        release = self.materialized_release()
        profile = yaml.safe_load(release["spec"]["profile"])
        profile["conditions"] = conditions
        release["spec"]["profile"] = yaml.safe_dump(profile)
        return release

    def canonical_conditions(self):
        return yaml.safe_load(PROFILE.read_text())["conditions"]

    def spec_only_conditions(self):
        conditions = self.canonical_conditions()
        conditions[0]["interface"].pop("operations", None)
        return conditions


class MaterializationTest(ResolverTestCase):
    def test_generated_profile_is_embedded_unchanged(self):
        release = self.materialized_release()
        self.assertEqual(release["spec"]["profile"], PROFILE.read_text())

    def test_materialized_release_name_is_parameterized(self):
        release = self.materialized_release("request-logger-breaking")
        self.assertEqual(release["metadata"]["name"], "request-logger-breaking")

    def test_materialized_release_resolves_to_platform_resources(self):
        output = resolver.resolve(self.materialized_release())
        by_kind = {doc["kind"]: doc for doc in output}

        self.assertEqual(by_kind["Redis"]["metadata"]["name"], "request-logger-cache")
        env = {
            var["name"]: var["value"]
            for var in by_kind["Deployment"]["spec"]["template"]["spec"]["containers"][0]["env"]
        }
        self.assertEqual(
            env,
            {
                "TODOS_API_URL": "http://todos-api.demo.svc.cluster.local:8080",
                "REDIS_HOST": "request-logger-cache.demo.svc.cluster.local",
                "REDIS_PORT": "6379",
            },
        )


class SupportEvaluationTest(ResolverTestCase):
    def test_rejects_unknown_condition_kind(self):
        conditions = self.canonical_conditions() + [
            {
                "name": "site-analytics",
                "kind": "google.analytics",
                "interface": {"type": "web"},
                "configuration": {"env": [{"property": "measurementId", "name": "GA_MEASUREMENT_ID"}]},
            }
        ]
        with self.assertRaises(resolver.UnsupportedConditionError) as ctx:
            resolver.resolve(self.release_with_conditions(conditions))
        self.assertEqual([c.name for c in ctx.exception.conditions], ["site-analytics"])
        self.assertIn("'google.analytics'", str(ctx.exception))

    def test_rejects_non_redis_cache_instead_of_skipping_it(self):
        conditions = self.canonical_conditions()
        conditions[1]["interface"]["engine"] = "memcached"
        with self.assertRaises(resolver.UnsupportedConditionError) as ctx:
            resolver.resolve(self.release_with_conditions(conditions))
        self.assertEqual([c.name for c in ctx.exception.conditions], ["conditions[1]"])

    def test_rejects_property_the_platform_cannot_provide(self):
        conditions = self.canonical_conditions()
        conditions[0]["configuration"]["env"][0]["property"] = "token"
        with self.assertRaises(resolver.UnsupportedConditionError) as ctx:
            resolver.resolve(self.release_with_conditions(conditions))
        self.assertIn("'token'", str(ctx.exception))

    def test_reports_every_unsupported_condition(self):
        conditions = self.canonical_conditions()
        conditions[0]["kind"] = "queue"
        conditions[1]["interface"]["engine"] = "memcached"
        with self.assertRaises(resolver.UnsupportedConditionError) as ctx:
            resolver.resolve(self.release_with_conditions(conditions))
        self.assertEqual(len(ctx.exception.conditions), 2)

    def test_falls_back_to_first_satisfiable_alternative(self):
        conditions = self.canonical_conditions()
        conditions[1]["configuration"]["alternatives"] = [
            {"env": [{"property": "url", "name": "REDIS_URL"}]},
        ]
        output = resolver.resolve(self.release_with_conditions(conditions))
        deployment = next(doc for doc in output if doc["kind"] == "Deployment")
        names = [var["name"] for var in deployment["spec"]["template"]["spec"]["containers"][0]["env"]]
        self.assertEqual(names, ["TODOS_API_URL", "REDIS_URL"])


class DeclaredAPISemanticsTest(ResolverTestCase):
    def test_operations_take_precedence_over_conflicting_spec(self):
        conditions = self.canonical_conditions()
        spec = conditions[0]["interface"]["spec"]
        spec["format"] = "asyncapi"
        spec["uri"] = "catalog://api/default/other-api"
        spec["version"] = ">9.0.0"

        output = resolver.resolve(self.release_with_conditions(conditions))
        deployment = next(doc for doc in output if doc["kind"] == "Deployment")
        env = {
            var["name"]: var["value"]
            for var in deployment["spec"]["template"]["spec"]["containers"][0]["env"]
        }
        self.assertEqual(env["TODOS_API_URL"], "http://todos-api.demo.svc.cluster.local:8080")


    def test_declared_uri_is_not_replaced_by_name_match(self):
        conditions = self.spec_only_conditions()
        conditions[0]["interface"]["spec"]["uri"] = "catalog://api/default/other-api"
        with self.assertRaises(resolver.ContractError) as ctx:
            resolver.resolve(self.release_with_conditions(conditions))
        self.assertIn("catalog://api/default/other-api", str(ctx.exception))

    def test_declared_version_must_match_published_version(self):
        conditions = self.spec_only_conditions()
        conditions[0]["interface"]["spec"]["version"] = "2.0.0"
        with self.assertRaises(resolver.ContractError) as ctx:
            resolver.resolve(self.release_with_conditions(conditions))
        self.assertIn("version: 2.0.0", str(ctx.exception))
        self.assertIn("version: 1.0.0", str(ctx.exception))

    def test_satisfied_version_constraint_resolves(self):
        for constraint in ("1.0.0", "=1.0.0", "^1.0.0", ">=1.0.0", "~1.0.0", "<2.0.0"):
            with self.subTest(constraint=constraint):
                conditions = self.spec_only_conditions()
                conditions[0]["interface"]["spec"]["version"] = constraint
                resolver.resolve(self.release_with_conditions(conditions))

    def test_unsatisfied_version_constraint_is_a_contract_error(self):
        conditions = self.spec_only_conditions()
        conditions[0]["interface"]["spec"]["version"] = ">1.0.0"
        with self.assertRaises(resolver.ContractError) as ctx:
            resolver.resolve(self.release_with_conditions(conditions))
        self.assertIn("version: >1.0.0", str(ctx.exception))

    def test_unparseable_version_requirement_is_unsupported(self):
        conditions = self.spec_only_conditions()
        conditions[0]["interface"]["spec"]["version"] = "1.0"
        with self.assertRaises(resolver.UnsupportedConditionError):
            resolver.resolve(self.release_with_conditions(conditions))

    def test_non_catalog_uri_is_unsupported(self):
        conditions = self.spec_only_conditions()
        conditions[0]["interface"]["spec"]["uri"] = "https://example.com/todos.openapi.yaml"
        with self.assertRaises(resolver.UnsupportedConditionError) as ctx:
            resolver.resolve(self.release_with_conditions(conditions))
        self.assertEqual([c.name for c in ctx.exception.conditions], ["conditions[0]"])

    def test_non_openapi_format_is_unsupported(self):
        conditions = self.spec_only_conditions()
        conditions[0]["interface"]["spec"]["format"] = "asyncapi"
        with self.assertRaises(resolver.UnsupportedConditionError):
            resolver.resolve(self.release_with_conditions(conditions))


class VersionConstraintTest(unittest.TestCase):
    def test_satisfies_version(self):
        cases = [
            ("1.0.0", "1.0.0", True),
            ("1.0.0", "1.0.1", False),
            ("=1.0.0", "1.0.0", True),
            (">1.0.0", "1.0.0", False),
            (">1.0.0", "1.0.1", True),
            (">=1.0.0", "1.0.0", True),
            (">=1.0.0", "0.9.9", False),
            ("<1.0.0", "1.0.0", False),
            ("<=1.0.0", "1.0.0", True),
            ("^1.0.0", "1.0.0", True),
            ("^1.0.0", "1.9.3", True),
            ("^1.0.0", "2.0.0", False),
            ("^1.2.0", "1.1.9", False),
            ("~1.0.0", "1.0.0", True),
            ("~1.0.0", "1.0.9", True),
            ("~1.0.0", "1.1.0", False),
            ("1.10.0", "1.9.0", False),
        ]
        for constraint, version, expected in cases:
            with self.subTest(constraint=constraint, version=version):
                self.assertIs(resolver.satisfies_version(constraint, version), expected)

    def test_rejects_unparseable_input(self):
        with self.assertRaises(ValueError):
            resolver.satisfies_version("1.0", "1.0.0")
        with self.assertRaises(ValueError):
            resolver.satisfies_version("1.0.0", "v1")


class ErrorCategoryTest(ResolverTestCase):
    def test_unnamed_conditions_resolve_without_rewriting_profile_bytes(self):
        release = self.materialized_release()
        original = release["spec"]["profile"]
        resolver.resolve(release)
        self.assertEqual(original, release["spec"]["profile"])

    def test_explicit_empty_or_duplicate_names_remain_invalid(self):
        for names in (("", "cache"), ("same", "same")):
            with self.subTest(names=names):
                conditions = self.canonical_conditions()
                for condition, name in zip(conditions, names):
                    condition["name"] = name
                with self.assertRaises(resolver.ProfileError):
                    resolver.resolve(self.release_with_conditions(conditions))

    def test_malformed_profile_is_a_profile_error(self):
        release = self.materialized_release()
        release["spec"]["profile"] = "apiVersion: v1\nkind: ConfigMap\nconditions:\n  - kind: api\n"
        with self.assertRaises(resolver.ProfileError):
            resolver.resolve(release)

    def test_missing_or_non_list_conditions_is_a_profile_error(self):
        header = "apiVersion: runtimeconditions.io/v1alpha1\nkind: RuntimeConditionsProfile\n"
        for label, tail in {
            "missing": "",
            "null": "conditions: null\n",
            "mapping": "conditions: {}\n",
            "string": 'conditions: ""\n',
        }.items():
            with self.subTest(conditions=label):
                release = self.materialized_release()
                release["spec"]["profile"] = header + tail
                with self.assertRaises(resolver.ProfileError) as ctx:
                    resolver.resolve(release)
                self.assertIn("conditions must be a list", str(ctx.exception))

    def test_empty_conditions_list_is_a_valid_envelope(self):
        release = self.materialized_release()
        release["spec"]["profile"] = (
            "apiVersion: runtimeconditions.io/v1alpha1\nkind: RuntimeConditionsProfile\nconditions: []\n"
        )
        kinds = [doc["kind"] for doc in resolver.resolve(release)]
        self.assertEqual(kinds, ["Deployment", "Service"])

    def test_breaking_catalog_still_reports_the_schema_incompatibility(self):
        self.use_catalog(BREAKING_CATALOG)
        with self.assertRaises(resolver.ContractError) as ctx:
            resolver.resolve(self.materialized_release())
        self.assertIn("response.completed: boolean", str(ctx.exception))


if __name__ == "__main__":
    unittest.main()
