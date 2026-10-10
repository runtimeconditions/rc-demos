"""Fast process-level tests; the separate KinD job proves Kubernetes ordering."""
import json
import os
import subprocess
import tempfile
import unittest
from pathlib import Path

import yaml
from prepare import prepare, ROOT, DEMO
from promise import promise
from assert_cluster import EXPECTED


class PlatformHandoffTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.temp = tempfile.TemporaryDirectory()
        cls.addClassCleanup(cls.temp.cleanup)
        cls.root = Path(cls.temp.name)
        cls.fixtures = cls.root / "fixtures"
        prepare(cls.fixtures)
        cls.gate = cls.root / "verify"
        subprocess.run(["go", "build", "-o", str(cls.gate), "./cmd/verify-kratix"], cwd=DEMO, check=True)
        cls.catalog = cls.root / "catalog"
        cls.catalog.mkdir()
        catalog = yaml.safe_load((ROOT / "kratix/manifests/catalog/todos-api-catalog.yaml").read_text())["data"]
        for name, content in catalog.items():
            (cls.catalog / name).write_text(content)

    def test_real_verifier_and_existing_resolver(self):
        for case, expected in EXPECTED.items():
            with self.subTest(case=case), tempfile.TemporaryDirectory() as scratch:
                work = Path(scratch)
                for name in ("verified", "metadata", "output"):
                    (work / name).mkdir()
                result = subprocess.run([
                    str(self.gate), "-input", str(self.fixtures / "requests" / (case + ".json")),
                    "-trusted-key", str(self.fixtures / "trusted.pub"),
                    "-output", str(work / "verified/object.yaml"), "-metadata", str(work / "metadata"),
                    "-termination-message", str(work / "gate-termination.json"),
                ], capture_output=True, text=True)
                status = json.loads((work / "gate-termination.json").read_text())
                self.assertEqual(expected, status["status"], result.stderr)
                if case not in ("valid", "unsupported"):
                    self.assertNotEqual(0, result.returncode)
                    self.assertFalse((work / "verified/object.yaml").exists())
                    continue
                self.assertEqual(0, result.returncode, result.stderr)
                verified = json.loads((work / "verified/object.yaml").read_text())
                original = json.loads((self.fixtures / "requests" / (case + ".json")).read_text())
                self.assertEqual(original["spec"]["profile"], verified["spec"]["profile"])
                self.assertNotIn("handoff", verified["spec"])
                run = self.run_resolver(work)
                status = yaml.safe_load((work / "metadata/status.yaml").read_text())
                self.assertTrue(status["handoff"]["handoff_verified"])
                if case == "unsupported":
                    self.assertEqual(1, run.returncode, run.stderr)
                    self.assertEqual("unsupported", status["consumer"]["status"])
                    self.assertTrue(status["unsupportedConditions"])
                    self.assertFalse(list((work / "output").iterdir()))
                else:
                    self.assertEqual(0, run.returncode, run.stderr)
                    docs = list(yaml.safe_load_all((work / "output/application-release.yaml").read_text()))
                    self.assertEqual({"Redis", "Deployment", "Service"}, {d["kind"] for d in docs})

    def run_resolver(self, work):
        env = dict(os.environ, PYTHONPATH=str(ROOT / "kratix/promises/application-release/pipeline"),
                   VERIFIED_INPUT_DIR=str(work / "verified"), KRATIX_METADATA_DIR=str(work / "metadata"),
                   KRATIX_OUTPUT_DIR=str(work / "output"), RUNTIME_CONDITIONS_CATALOG_DIR=str(self.catalog),
                   TERMINATION_MESSAGE_PATH=str(work / "resolver-termination.json"))
        return subprocess.run(["python3", str(DEMO / "kratix/resolve.py")], env=env, capture_output=True, text=True)

    def test_no_unverified_fallback(self):
        with tempfile.TemporaryDirectory() as scratch:
            work = Path(scratch)
            for name in ("verified", "metadata", "output"):
                (work / name).mkdir()
            (work / "metadata/status.yaml").write_text('{"handoff":{"handoff_verified":true}}')
            run = self.run_resolver(work)
            self.assertNotEqual(0, run.returncode)
            self.assertIn("no raw-input fallback", run.stderr)
            self.assertFalse(list((work / "output").iterdir()))

    def test_promise_boundary(self):
        doc = promise()
        pipeline = doc["spec"]["workflows"]["resource"]["configure"][0]["spec"]
        self.assertEqual(["verify-handoff", "resolver"], [c["name"] for c in pipeline["containers"]])
        mounts = pipeline["containers"][1]["volumeMounts"]
        self.assertEqual([{"name": "verified-request", "mountPath": "/handoff/verified", "readOnly": True}], mounts)
        self.assertEqual("Never", pipeline["restartPolicy"])
        self.assertNotIn("signer.key", json.dumps(doc))
        self.assertNotIn("secret", json.dumps(doc).lower())

    def test_verifier_does_not_link_validator(self):
        deps = subprocess.check_output(["go", "list", "-deps", "./cmd/verify-kratix"], cwd=DEMO, text=True)
        for forbidden in ("go-rc-profiler", "jsonschema", "/internal/producer"):
            self.assertNotIn(forbidden, deps)


if __name__ == "__main__":
    unittest.main()
