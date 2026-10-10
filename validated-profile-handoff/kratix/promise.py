"""Render an opt-in Promise variant without changing the portable-profile demo."""
from pathlib import Path
import yaml

ROOT = Path(__file__).resolve().parents[2]


def promise():
    doc = yaml.safe_load((ROOT / "kratix/manifests/promises/application-release.yaml").read_text())
    doc["metadata"]["name"] = "validated-application-release"
    crd = doc["spec"]["api"]
    crd["metadata"]["name"] = "validatedapplicationreleases.platform.demoteam.io"
    crd["spec"]["names"] = {
        "kind": "ValidatedApplicationRelease", "plural": "validatedapplicationreleases",
        "singular": "validatedapplicationrelease", "shortNames": ["var"],
    }
    root = crd["spec"]["versions"][0]["schema"]["openAPIV3Schema"]["properties"]
    root["status"] = {"type": "object", "x-kubernetes-preserve-unknown-fields": True}
    schema = root["spec"]
    schema["required"].append("handoff")
    schema["properties"]["handoff"] = {
        "type": "object", "required": ["evidence", "signature", "extensions"],
        "properties": {
            "evidence": {"type": "string"},
            "signature": {"type": "string"},
            "extensions": {"type": "object", "additionalProperties": {"type": "string"}},
        },
    }
    pipeline = doc["spec"]["workflows"]["resource"]["configure"][0]
    # Kratix includes the pipeline name in generated ServiceAccount names and
    # limits the resulting name to 60 characters. Keep this short enough for
    # validated-application-release resource workflows.
    pipeline["metadata"]["name"] = "resolve"
    pipeline["spec"].update({
        "restartPolicy": "Never", "jobOptions": {"backoffLimit": 0},
        "volumes": [
            {"name": "verified-request", "emptyDir": {}},
            {"name": "handoff-trust", "configMap": {"name": "handoff-trust"}},
        ],
        "containers": [
            {
                "name": "verify-handoff", "image": "rc-handoff-verifier:experiment",
                "imagePullPolicy": "Never",
                "volumeMounts": [
                    {"name": "verified-request", "mountPath": "/handoff/verified"},
                    {"name": "handoff-trust", "mountPath": "/trust", "readOnly": True},
                ],
            },
            {
                "name": "resolver", "image": "rc-handoff-resolver:experiment",
                "imagePullPolicy": "Never",
                "volumeMounts": [
                    {"name": "verified-request", "mountPath": "/handoff/verified", "readOnly": True},
                ],
            },
        ],
    })
    return doc


if __name__ == "__main__":
    print(yaml.safe_dump(promise(), sort_keys=False))
