"""Create the GitHub release only after anonymous verification; never replace assets."""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
from package import ROOT, digest, receipt

def verify_published(path, accepted):
    verified = json.loads(path.read_text())
    for key in ("source", "version", "oracle_sha256"):
        assert verified[key] == accepted[key], f"verification {key} differs"
    assert verified["mode"] == "published"
    consumers = verified["anonymous_consumers"]
    assert len(consumers) == 3 and set(consumers) == {"npm", "crates.io", "Go proxy"}
    assert verified["go_origin"]["Hash"] == accepted["source"]
    assert verified["ci"] and str(verified["run_id"]).isdigit()


def preserve_existing(directory, accepted, repo):
    immutable = [directory / p["file"] for p in accepted["packages"]]
    immutable.append(directory / "candidate.json")
    proof = directory / "published-verification.json"
    verify_published(proof, accepted)
    with tempfile.TemporaryDirectory() as temp:
        subprocess.run(["gh", "release", "download", "v0.11.1", "--repo", repo, "--dir", temp], check=True)
        for asset in immutable:
            assert digest(Path(temp) / asset.name) == digest(asset), "immutable release asset differs"
        original = Path(temp) / proof.name
        if original.exists():
            verify_published(original, accepted)
        else:
            # A previous create can succeed before its final asset upload fails.
            # Add only the missing receipt, never replace an existing asset.
            subprocess.run(["gh", "release", "upload", "v0.11.1", str(proof), "--repo", repo], check=True)
            subprocess.run(["gh", "release", "download", "v0.11.1", "--repo", repo,
                            "--pattern", proof.name, "--dir", temp], check=True)
            assert digest(original) == digest(proof), "uploaded verification receipt differs"
            verify_published(original, accepted)
    print("Existing release assets and complete published verification receipt verified")


def main(directory):
    accepted = receipt(directory)
    verify_published(directory / "published-verification.json", accepted)
    repo = os.environ["GITHUB_REPOSITORY"]
    assets = [directory / p["file"] for p in accepted["packages"]]
    assets += [directory / "candidate.json", directory / "published-verification.json"]
    existing = subprocess.run(["gh", "api", f"repos/{repo}/releases/tags/v0.11.1"], text=True, capture_output=True)
    if existing.returncode == 0:
        release = json.loads(existing.stdout)
        assert release["tag_name"] == "v0.11.1" and not release["draft"]
        preserve_existing(directory, accepted, repo)
    else:
        assert "404" in existing.stderr, existing.stderr
        subprocess.run(["gh", "release", "create", "v0.11.1", "--repo", repo, "--verify-tag", "--title", "horos 0.11.1",
                        "--notes-file", str(ROOT / "docs/releases/v0.11.1.md"), *map(str, assets)], check=True)


if __name__ == "__main__":
    main(Path(sys.argv[1]).resolve())
