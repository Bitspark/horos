"""Prepare and compare horos release artifacts. Registry reads are anonymous."""
from pathlib import Path
import argparse
import base64
import hashlib
import json
import os
import shutil
import subprocess
import time
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parent.parent
VERSION = "0.11.1"


def run(args, cwd=ROOT):
    return subprocess.check_output(args, cwd=cwd, text=True).strip()


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def get(url):
    request = urllib.request.Request(url, headers={"User-Agent": "horos-release (https://github.com/Bitspark/horos)"})
    for attempt in range(5):
        try:
            with urllib.request.urlopen(request, timeout=60) as response:
                return response.read()
        except urllib.error.HTTPError as error:
            if error.code == 404:
                return None
            if error.code not in (429, 500, 502, 503, 504) or attempt == 4:
                raise
            time.sleep(2 ** attempt)
    raise AssertionError("unreachable")


def receipt(directory):
    data = json.loads((directory / "candidate.json").read_text())
    assert data["schema"] == 1 and data["version"] == VERSION and data["mode"] == "rehearse"
    assert data["source"] == run(["git", "rev-parse", "HEAD"]), "candidate/source mismatch"
    assert data["tree"] == run(["git", "rev-parse", "HEAD^{tree}"]), "candidate/tree mismatch"
    assert data["tags"] == [f"v{VERSION}", f"core/go/v{VERSION}"]
    expected = {("npm", "@bitspark/horos", VERSION, f"bitspark-horos-{VERSION}.tgz"),
                ("crates.io", "bitspark-horos", VERSION, f"bitspark-horos-{VERSION}.crate")}
    assert len(data["packages"]) == 2
    assert {(p["registry"], p["name"], p["version"], p["file"]) for p in data["packages"]} == expected
    assert data["oracle_sha256"] == digest(ROOT / "vectors/conformance.json")
    for package in data["packages"]:
        assert Path(package["file"]).name == package["file"]
        assert digest(directory / package["file"]) == package["sha256"], package["name"]
        archive = (directory / package["file"]).read_bytes()
        assert len(archive) == package["size"]
        if package["registry"] == "npm":
            assert package["integrity"] == "sha512-" + base64.b64encode(hashlib.sha512(archive).digest()).decode()
    return data


def prepare(directory):
    directory.mkdir(parents=True, exist_ok=True)
    assert not list(directory.iterdir()), "candidate directory must be empty"
    assert not run(["git", "status", "--porcelain"]), "source is dirty"
    pkg = json.loads((ROOT / "core/ts/package.json").read_text())
    assert pkg["version"] == VERSION and pkg["name"] == "@bitspark/horos"
    run(["npm", "ci", "--ignore-scripts", "--registry=https://registry.npmjs.org/"], ROOT / "core/ts")
    run(["npm", "run", "build"], ROOT / "core/ts")
    packed = json.loads(run(["npm", "pack", "--json", "--pack-destination", str(directory)], ROOT / "core/ts"))
    assert len(packed) == 1
    npm = packed[0]
    assert not any(any(s in f["path"] for s in ("semantics", "witness", "/wire.")) for f in npm["files"])
    run(["cargo", "package", "--locked", "-p", "bitspark-horos"])
    crate = ROOT / f"target/package/bitspark-horos-{VERSION}.crate"
    shutil.copyfile(crate, directory / crate.name)
    data = {
        "schema": 1, "mode": "rehearse", "version": VERSION,
        "source": run(["git", "rev-parse", "HEAD"]),
        "tree": run(["git", "rev-parse", "HEAD^{tree}"]),
        "run_id": os.environ["GITHUB_RUN_ID"],
        "oracle_sha256": digest(ROOT / "vectors/conformance.json"),
        "tags": [f"v{VERSION}", f"core/go/v{VERSION}"],
        "packages": [
            {"registry": "npm", "name": "@bitspark/horos", "version": VERSION, "file": npm["filename"], "sha256": digest(directory / npm["filename"]), "integrity": npm["integrity"], "size": (directory / npm["filename"]).stat().st_size},
            {"registry": "crates.io", "name": "bitspark-horos", "version": VERSION, "file": crate.name, "sha256": digest(directory / crate.name), "size": (directory / crate.name).stat().st_size},
        ],
        "go": {"module": "github.com/Bitspark/horos/core/go", "version": "v" + VERSION},
        "python": "source-only validation",
    }
    (directory / "candidate.json").write_text(json.dumps(data, indent=2) + "\n")
    run(["npm", "publish", str(directory / npm["filename"]), "--dry-run", "--access", "public", "--provenance=false", "--registry=https://registry.npmjs.org/"])
    run(["cargo", "publish", "--dry-run", "--locked", "-p", "bitspark-horos"])
    print(json.dumps({"source": data["source"], "artifacts": [p["file"] for p in data["packages"]]}))


def registry(directory, required, wait):
    data = receipt(directory)
    results = []
    for package in data["packages"]:
        url = f"https://registry.npmjs.org/@bitspark%2fhoros/{VERSION}" if package["registry"] == "npm" else f"https://crates.io/api/v1/crates/bitspark-horos/{VERSION}"
        raw = None
        for attempt in range(30 if wait else 1):
            raw = get(url)
            if raw is not None:
                break
            if wait:
                time.sleep(10)
        if raw is None:
            assert not required, f"{package['name']} not served"
            results.append({"name": package["name"], "status": "absent"})
            continue
        meta = json.loads(raw)
        if package["registry"] == "npm":
            assert meta["name"] == package["name"] and meta["version"] == VERSION
            assert meta["dist"]["integrity"] == package["integrity"], "npm immutable version differs"
            url = meta["dist"]["tarball"]
            assert url.startswith("https://registry.npmjs.org/"), url
        else:
            assert meta["version"]["num"] == VERSION
            assert meta["version"]["checksum"] == package["sha256"], "crate immutable version differs"
            url = f"https://crates.io/api/v1/crates/bitspark-horos/{VERSION}/download"
        archive = get(url)
        assert archive is not None and hashlib.sha256(archive).hexdigest() == package["sha256"], "served artifact differs"
        results.append({"name": package["name"], "status": "identical", "sha256": package["sha256"]})
    (directory / "registry.json").write_text(json.dumps(results, indent=2) + "\n")
    print(json.dumps(results))


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("mode", choices=["prepare", "check", "registry", "reproduce-crate"])
    parser.add_argument("directory", type=Path)
    parser.add_argument("--required", action="store_true")
    parser.add_argument("--wait", action="store_true")
    args = parser.parse_args()
    directory = args.directory.resolve()
    if args.mode == "prepare":
        prepare(directory)
    elif args.mode == "registry":
        registry(directory, args.required, args.wait)
    else:
        data = receipt(directory)
        if args.mode == "reproduce-crate":
            run(["cargo", "package", "--locked", "-p", "bitspark-horos"])
            expected = next(p for p in data["packages"] if p["registry"] == "crates.io")
            assert digest(ROOT / "target/package" / expected["file"]) == expected["sha256"], "Cargo did not reproduce the candidate"
        print("candidate matches exact source and artifact hashes")


if __name__ == "__main__":
    main()
