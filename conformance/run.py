"""Compare built native/TypeScript CLIs with the oracles or a seeded sample."""
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
    return next(iter(results.values()))


def check_located(args, expected=None):
    """Every core prints the same diagnosis JSON (ADR 0009), equal to the expectation if given."""
    results = {}
    for name, command in CORES.items():
        result = subprocess.run(command + args, capture_output=True, text=True, timeout=30, check=True)
        results[name] = json.loads(result.stdout)
    first = next(iter(results.values()))
    if any(r != first for r in results.values()) or (expected is not None and first != expected):
        raise AssertionError({"arguments": args, "expected": expected, "results": results})
    return first


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
        located = json.loads((ROOT / "vectors/diagnose.json").read_text())
        for case in located["cases"]:
            check_located(["diagnose-wf", case["descriptor"]], case["wf"])
            checks += 1
            for value in case["values"]:
                check_located(["diagnose", case["descriptor"], value["value"]], value["diagnosis"])
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
            conforms = check(["conforms", descriptor, value])
            diagnosis = check_located(["diagnose", descriptor, value])
            completed = {"conforms": True, "mismatch": False, "not_well_formed": False}.get(diagnosis["verdict"])
            if completed is not None and completed != conforms:
                raise AssertionError({"arguments": [descriptor, value], "conforms": conforms, "diagnosis": diagnosis})
            checks += 3
    print(f"{checks} judgments agree across {len(CORES)} cores; seed={args.seed}", flush=True)


if __name__ == "__main__":
    main()
