"""Compare built native/TypeScript CLIs with the oracle or a seeded sample."""
from pathlib import Path
import argparse
import json
import subprocess

ROOT = Path(__file__).resolve().parent.parent
CORES = {
    "rust": [str(ROOT / "target/release/horos-conform")],
    "go": [str(ROOT / "conformance/.bin/horos-conform-go")],
    "typescript": ["node", str(ROOT / "core/ts/dist/src/cli.js")],
}


def check(args, expected=None):
    results = {}
    for name, command in CORES.items():
        result = subprocess.run(command + args, capture_output=True, text=True, timeout=30, check=True)
        result = result.stdout.strip()
        if result not in {"true", "false"}:
            raise AssertionError((name, args, result))
        results[name] = result == "true"
    if len(set(results.values())) != 1 or (expected is not None and next(iter(results.values())) != expected):
        raise AssertionError({"arguments": args, "expected": expected, "results": results})


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--seed", type=int)
    parser.add_argument("--count", type=int, default=2000)
    args = parser.parse_args()
    checks = 0
    if args.seed is None:
        doc = json.loads((ROOT / "vectors/conformance.json").read_text())
        for case in doc["cases"]:
            check(["wf", case["descriptor"]], case["wf"])
            checks += 1
            for value in case["values"]:
                check(["conforms", case["descriptor"], value["value"]], value["conforms"])
                checks += 1
    else:
        if args.count <= 0:
            raise SystemExit("count must be positive")
        generated = subprocess.run([str(ROOT / "target/release/horos-fuzz"), "--seed", str(args.seed), "--count", str(args.count)], capture_output=True, text=True, timeout=90, check=True)
        lines = generated.stdout.splitlines()
        if len(lines) != args.count:
            raise AssertionError("generator count mismatch")
        for line in lines:
            descriptor, value = line.split()
            check(["wf", descriptor])
            check(["conforms", descriptor, value])
            checks += 2
    print(f"{checks} judgments agree across {len(CORES)} cores; seed={args.seed}", flush=True)


if __name__ == "__main__":
    main()
