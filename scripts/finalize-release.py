"""Create the GitHub release only after anonymous verification; never replace assets."""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
from package import ROOT, digest, receipt

directory = Path(sys.argv[1]).resolve()
accepted = receipt(directory)
verified = json.loads((directory / "published-verification.json").read_text())
assert verified["source"] == accepted["source"] and verified["mode"] == "published"
repo = os.environ["GITHUB_REPOSITORY"]
assets = [directory / p["file"] for p in accepted["packages"]]
assets += [directory / "candidate.json", directory / "published-verification.json"]
existing = subprocess.run(["gh", "api", f"repos/{repo}/releases/tags/v0.6.0"], text=True, capture_output=True)
if existing.returncode == 0:
    release = json.loads(existing.stdout)
    assert release["tag_name"] == "v0.6.0" and not release["draft"]
    with tempfile.TemporaryDirectory() as temp:
        subprocess.run(["gh", "release", "download", "v0.6.0", "--repo", repo, "--dir", temp], check=True)
        for asset in assets[:3]:
            assert digest(Path(temp) / asset.name) == digest(asset), "immutable release asset differs"
    print("Existing release and accepted artifact assets verified; original verification receipt preserved")
else:
    assert "404" in existing.stderr, existing.stderr
    subprocess.run(["gh", "release", "create", "v0.6.0", "--repo", repo, "--verify-tag", "--title", "horos 0.6.0",
                    "--notes-file", str(ROOT / "docs/releases/v0.6.0.md"), *map(str, assets)], check=True)
