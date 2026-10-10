"""Use the existing resolver, but only with the gate's verified request volume."""
import json
import os
from pathlib import Path

import yaml
import resolver


def main():
    resolver.INPUT_DIR = Path(os.environ.get("VERIFIED_INPUT_DIR", "/handoff/verified"))
    status_path = resolver.METADATA_DIR / "status.yaml"
    prior = yaml.safe_load(status_path.read_text())
    handoff = prior.get("handoff", {})
    if not handoff.get("handoff_verified") or not (resolver.INPUT_DIR / "object.yaml").is_file():
        raise RuntimeError("verified handoff required; no raw-input fallback")
    code = resolver.main()
    status = yaml.safe_load(status_path.read_text())
    status["handoff"] = handoff
    category = "supported"
    if "unsupportedConditions" in status:
        category = "unsupported"
    elif "invalidProfile" in status:
        category = "profile_unusable"
    elif "validationError" in status:
        category = "contract_incompatible"
    elif code:
        category = "fulfillment_failed"
    status["consumer"] = {"status": category}
    status_path.write_text(yaml.safe_dump(status))
    Path(os.environ.get("TERMINATION_MESSAGE_PATH", "/dev/termination-log")).write_text(
        json.dumps({"status": category, "handoff_verified": True})
    )
    print(json.dumps(status), flush=True)
    return code


if __name__ == "__main__":
    raise SystemExit(main())
