"""Prepare real upstream-signed requests. Never place private keys in output."""
import argparse
import base64
import copy
import json
import subprocess
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
DEMO = ROOT / "validated-profile-handoff"


def request(bundle, name):
    return {
        "apiVersion": "platform.demoteam.io/v1alpha1", "kind": "ValidatedApplicationRelease",
        "metadata": {"name": name, "namespace": "demo"},
        "spec": {
            "image": "ghcr.io/runtimeconditions/request-logger:latest", "imagePullPolicy": "IfNotPresent",
            "profile": (bundle / "profile.yaml").read_text(),
            "catalog": {"configMapRef": {"name": "platform-api-catalog", "namespace": "platform-demo-system"}},
            "handoff": {
                "evidence": (bundle / "evidence.json").read_text(),
                "signature": base64.b64encode((bundle / "evidence.sig").read_bytes()).decode(),
                "extensions": {p.stem: p.read_text() for p in (bundle / "extensions").glob("*.yaml")},
            },
        },
    }


def prepare(out):
    out.mkdir(parents=True, exist_ok=False)
    (out / "requests").mkdir()
    with tempfile.TemporaryDirectory(prefix="handoff-producer-") as private:
        private = Path(private)
        for command in ("validate", "keygen"):
            subprocess.run(["go", "build", "-o", str(private / command), "./cmd/" + command], cwd=DEMO, check=True)
        for name in ("trusted", "rogue"):
            subprocess.run([str(private / "keygen"), "-out", str(private / name)], check=True)
        (out / "trusted.pub").write_bytes((private / "trusted/trusted.pub").read_bytes())
        profile = ROOT / "artifacts/request-logger-http.profile.yaml"
        unsupported = private / "unsupported.yaml"
        unsupported.write_bytes(profile.read_bytes().replace(b"engine: redis", b"engine: memcached", 1))
        requests = {}
        for name, source, signer in (("valid", profile, "trusted"), ("unsupported", unsupported, "trusted"), ("untrusted", profile, "rogue")):
            bundle = out / name
            subprocess.run([str(private / "validate"), "-profile", str(source), "-catalog", str(ROOT.parent / "extensions/catalog"),
                            "-key", str(private / signer / "signer.key"), "-out", str(bundle)], check=True)
            requests[name] = request(bundle, "handoff-" + name)
        for name in ("profile-changed", "extension-changed"):
            requests[name] = copy.deepcopy(requests["valid"])
            requests[name]["metadata"]["name"] = "handoff-" + name
        requests["profile-changed"]["spec"]["profile"] += "\n# changed after signing\n"
        extensions = requests["extension-changed"]["spec"]["handoff"]["extensions"]
        extensions[sorted(extensions)[0]] += "\n# changed after signing\n"
        for name, data in requests.items():
            (out / "requests" / (name + ".json")).write_text(json.dumps(data, indent=2))


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--out", type=Path, required=True)
    prepare(parser.parse_args().out.resolve())
