"""Check Go-materialised ground descriptors with the TypeScript checker (issue #36).

The Go declarations test materialises the descriptors through
Program.GroundDescriptor, asserts the Go checker against judgments authored from
the specification and writes them out. Each TypeScript lane installs a package
in a fresh consumer and must reach the same judgments:

- in-repo: the package packed from core/ts (built by `check.sh conformance`),
  so a change to either implementation is caught before release;
- published: the npm release Bittype's binding design consumes today. Its
  version is pinned so this check stays reproducible; move the pin when a new
  npm release is published.

Each lane installs only its package, so Ontos comes from that package's own
exact dependency pins: the in-repo lane gets the Ontos version core/ts requires,
and the published lane gets the one its release requires (0.16.0 for 0.9.0).
ground-evidence.mjs checks that exactly one Ontos family is installed.
"""
from pathlib import Path
import json
import shutil
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parent.parent
PUBLISHED = "@bitspark/horos@0.9.0"


def run(args, cwd):
    result = subprocess.run(args, cwd=cwd, text=True, capture_output=True, timeout=600)
    if result.returncode != 0:
        raise RuntimeError(f"{' '.join(map(str, args))} failed ({result.returncode})\n{result.stdout}\n{result.stderr}")
    return result.stdout


def main():
    with tempfile.TemporaryDirectory(prefix="horos-ground-") as temp:
        root = Path(temp)
        fixture = root / "ground-evidence.json"
        run(["go", "test", "-mod=readonly", "-count=1", "-run", "^TestGroundDescriptorEvidence$",
             "./declarations", "-args", f"-ground-evidence={fixture}"], ROOT / "core/go")
        if not json.loads(fixture.read_text())["cases"]:
            raise AssertionError("the Go test wrote no cases")
        packed = json.loads(run(["npm", "pack", "--json", "--pack-destination", str(root)], ROOT / "core/ts"))
        assert len(packed) == 1
        for lane, target in (("in-repo", str(root / packed[0]["filename"])), ("published", PUBLISHED)):
            consumer = root / lane
            consumer.mkdir()
            (consumer / "package.json").write_text('{"name":"horos-ground-consumer","private":true,"type":"module"}')
            run(["npm", "install", "--ignore-scripts", "--no-audit", "--no-fund", target], consumer)
            shutil.copyfile(ROOT / "conformance/ground-evidence.mjs", consumer / "ground-evidence.mjs")
            print(run(["node", "ground-evidence.mjs", str(fixture), lane], consumer), end="", flush=True)


if __name__ == "__main__":
    main()
