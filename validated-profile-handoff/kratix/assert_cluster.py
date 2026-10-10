"""Assert real Kratix-created Pod ordering, rejection status, and emitted Work."""
import argparse
import base64
import gzip
import json
import subprocess
import time
from pathlib import Path

import yaml

EXPECTED = {"valid": "verified", "unsupported": "verified", "profile-changed": "profile_digest_mismatch",
            "extension-changed": "extension_mismatch", "untrusted": "evidence_untrusted"}


def kubectl(*args):
    return subprocess.check_output(["kubectl", *args], text=True)


def objects(kind, case):
    selector = f"kratix.io/promise-name=validated-application-release,kratix.io/resource-name=handoff-{case}"
    return json.loads(kubectl("get", kind, "-A", "-l", selector, "-o", "json"))["items"]


def inspect_pod(pod, case):
    names = [c["name"] for c in pod["spec"]["initContainers"]]
    assert names.index("verify-handoff") < names.index("resolver"), names
    states = {c["name"]: c for c in pod["status"].get("initContainerStatuses", [])}
    gate = states["verify-handoff"]["state"]["terminated"]
    result = json.loads(gate["message"])
    assert result["status"] == EXPECTED[case], result
    assert result["support_evaluated"] is False, result
    resolver = states.get("resolver", {})
    if case not in ("valid", "unsupported"):
        assert gate["exitCode"] != 0 and not result["handoff_verified"], gate
        assert not resolver.get("containerID"), resolver
        assert not resolver.get("lastState"), resolver
        assert "terminated" not in resolver.get("state", {}) and "running" not in resolver.get("state", {}), resolver
        assert pod["status"]["phase"] == "Failed", pod["status"]
    else:
        assert gate["exitCode"] == 0 and result["handoff_verified"], gate
        done = resolver["state"]["terminated"]
        consumed = json.loads(done["message"])
        expected = "supported" if case == "valid" else "unsupported"
        assert consumed["status"] == expected and consumed["handoff_verified"], consumed
        assert done["exitCode"] == (0 if case == "valid" else 1), done
        assert pod["status"]["phase"] == ("Succeeded" if case == "valid" else "Failed")
    return result


def emitted_documents(works):
    for work in works:
        for group in work["spec"].get("workloadGroups", []):
            for workload in group.get("workloads", []):
                data = gzip.decompress(base64.b64decode(workload["content"]))
                yield from (doc for doc in yaml.safe_load_all(data) if doc)


def check_case(case, diagnostics):
    deadline = time.monotonic() + 300
    while time.monotonic() < deadline:
        pods = objects("pods", case)
        if pods and all(p.get("status", {}).get("phase") in ("Succeeded", "Failed") for p in pods):
            break
        time.sleep(2)
    else:
        raise AssertionError(f"{case}: workflow did not reach terminal state within 300s")
    # Check every attempt, not just the newest Pod, so retries cannot hide a bypass.
    for pod in pods:
        inspect_pod(pod, case)
    (diagnostics / (case + "-pods.json")).write_text(json.dumps(pods, indent=2))
    works = objects("works", case)
    (diagnostics / (case + "-works.json")).write_text(json.dumps(works, indent=2))
    if case == "valid":
        docs = list(emitted_documents(works))
        assert {d["kind"] for d in docs} >= {"Deployment", "Service", "Redis"}, docs
        deployment = next(d for d in docs if d["kind"] == "Deployment")
        assert deployment["metadata"]["name"] == "handoff-valid", deployment
        env = {e["name"]: e["value"] for e in deployment["spec"]["template"]["spec"]["containers"][0]["env"]}
        assert env["TODOS_API_URL"] == "http://todos-api.demo.svc.cluster.local:8080", env
        assert env["REDIS_HOST"] == "handoff-valid-cache.demo.svc.cluster.local", env
        assert env["REDIS_PORT"] == "6379", env
        (diagnostics / "emitted-resources.yaml").write_text(yaml.safe_dump_all(docs))
    else:
        assert not works, f"{case}: rejected demand emitted Work: {works}"
    print(f"PASS {case}: {EXPECTED[case]}", flush=True)


def collect(diagnostics):
    diagnostics.mkdir(parents=True, exist_ok=True)
    # No Secrets or kubeconfig. Request artifacts contain only public evidence.
    for kind in ("pods", "jobs", "works", "events", "promises", "validatedapplicationreleases"):
        result = subprocess.run(["kubectl", "get", kind, "-A", "-o", "yaml"], capture_output=True, text=True)
        (diagnostics / (kind + ".yaml")).write_text(result.stdout + result.stderr)
    result = subprocess.run(["kubectl", "get", "pods", "-A", "-o", "json"], capture_output=True, text=True)
    if result.returncode:
        return
    for pod in json.loads(result.stdout)["items"]:
        namespace, name = pod["metadata"]["namespace"], pod["metadata"]["name"]
        for c in pod["spec"].get("initContainers", []) + pod["spec"].get("containers", []):
            logs = subprocess.run(["kubectl", "logs", "-n", namespace, name, "-c", c["name"], "--tail=300"], capture_output=True, text=True, timeout=30)
            (diagnostics / f"{namespace}-{name}-{c['name']}.log").write_text(logs.stdout + logs.stderr)


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--diagnostics", type=Path, required=True)
    parser.add_argument("--collect-only", action="store_true")
    args = parser.parse_args()
    args.diagnostics.mkdir(parents=True, exist_ok=True)
    if args.collect_only:
        collect(args.diagnostics)
    else:
        try:
            for case in EXPECTED:
                check_case(case, args.diagnostics)
        finally:
            collect(args.diagnostics)
