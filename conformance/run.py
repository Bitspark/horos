"""Compare the built Go, Rust and TypeScript CLIs and the Python CLI with the oracles or a seeded sample.

Four implementations answer every judgment through the same command-line contract
(`wf`, `conforms`, `diagnose-wf`, `diagnose`); every answer must be identical, and equal
to the authored expectation where an oracle gives one. The Python CLI needs
`ONTOS_PY_ROOT`, like the Python lane; it is not skipped when that is missing.

    python3 conformance/run.py                          # the authored oracles
    python3 conformance/run.py --seed 7 --count 2000    # a seeded horos-fuzz sample
    python3 conformance/run.py --seed 7 --jobs 4        # judgments on 4 threads

Every disagreement is printed as one JSON line with its reproducing input before the run
fails, so a seeded run reports all of its findings at once. A command that runs longer than
`--timeout` seconds is reported as a finding too.
"""

from concurrent.futures import ThreadPoolExecutor
from pathlib import Path
import argparse
import json
import os
import subprocess
import sys
import time

ROOT = Path(__file__).resolve().parent.parent
CORES = {
    "rust": [str(ROOT / "target/release/horos-conform")],
    "go": [str(ROOT / "conformance/.bin/horos-conform-go")],
    "typescript": ["node", str(ROOT / "core/ts/dist/src/cli.js")],
    "python": [sys.executable, "-I", str(ROOT / "core/py/horos-conform.py")],
}


class Disagreement(Exception):
    pass


TIMEOUT = 120.0


def answers(args):
    results = {}
    for name, command in CORES.items():
        try:
            result = subprocess.run(command + args, capture_output=True, text=True, timeout=TIMEOUT)
        except subprocess.TimeoutExpired:
            raise Disagreement({"arguments": args, "core": name, "timeout": TIMEOUT}) from None
        if result.returncode != 0:
            raise Disagreement({"arguments": args, "core": name, "exit": result.returncode, "stderr": result.stderr.strip()})
        results[name] = result.stdout.strip()
    return results


def check(args, expected=None):
    results = answers(args)
    if any(r not in {"true", "false"} for r in results.values()):
        raise Disagreement({"arguments": args, "results": results})
    results = {name: r == "true" for name, r in results.items()}
    first = next(iter(results.values()))
    if len(set(results.values())) != 1 or (expected is not None and first != expected):
        raise Disagreement({"arguments": args, "expected": expected, "results": results})
    return first


def check_located(args, expected=None):
    """Every core prints the same diagnosis JSON (ADR 0009), equal to the expectation if given."""
    results = {name: json.loads(r) for name, r in answers(args).items()}
    first = next(iter(results.values()))
    if any(r != first for r in results.values()) or (expected is not None and first != expected):
        raise Disagreement({"arguments": args, "expected": expected, "results": results})
    return first


def one(checker, *arguments):
    """Run one check and count it as one judgment."""
    checker(*arguments)
    return 1


def oracle_units():
    """One unit per authored judgment: a thunk returning the number of judgments it checked."""
    for oracle in ("conformance.json", "integer-bounds.json", "recursion-scope.json", "checking-work.json"):
        doc = json.loads((ROOT / "vectors" / oracle).read_text())
        for case in doc["cases"]:
            yield lambda c=case: one(check, ["wf", c["descriptor"]], c["wf"])
            for value in case["values"]:
                yield lambda c=case, v=value: one(check, ["conforms", c["descriptor"], v["value"]], v["conforms"])
    # The location corpus's wf is a diagnosis; the ref-scoping oracle's wf is the
    # boolean checked above and its wf diagnosis is diagnose_wf (ADR 0011).
    for corpus, wf_key in (
        ("diagnose.json", "wf"),
        ("recursion-scope.json", "diagnose_wf"),
        ("checking-work.json", "diagnose_wf"),
    ):
        located = json.loads((ROOT / "vectors" / corpus).read_text())
        for case in located["cases"]:
            yield lambda c=case, k=wf_key: one(check_located, ["diagnose-wf", c["descriptor"]], c[k])
            for value in case["values"]:
                yield lambda c=case, v=value: one(check_located, ["diagnose", c["descriptor"], v["value"]], v["diagnosis"])


def pair_unit(descriptor, value):
    def unit():
        check(["wf", descriptor])
        conforms = check(["conforms", descriptor, value])
        diagnosis = check_located(["diagnose", descriptor, value])
        completed = {"conforms": True, "mismatch": False, "not_well_formed": False}.get(diagnosis["verdict"])
        if completed is not None and completed != conforms:
            raise Disagreement({"arguments": [descriptor, value], "conforms": conforms, "diagnosis": diagnosis})
        return 3

    return unit


def run_units(units, jobs):
    """Run every unit; return the judgments checked, the findings and the slowest unit's seconds."""

    def guarded(unit):
        start = time.monotonic()
        try:
            return unit(), None, time.monotonic() - start
        except Disagreement as finding:
            return 0, finding.args[0], time.monotonic() - start

    with ThreadPoolExecutor(max_workers=jobs) as pool:
        outcomes = list(pool.map(guarded, units))
    findings = [f for _, f, _ in outcomes if f is not None]
    return sum(n for n, _, _ in outcomes), findings, max((t for _, _, t in outcomes), default=0.0)


def main():
    global TIMEOUT
    parser = argparse.ArgumentParser()
    parser.add_argument("--seed", type=int)
    parser.add_argument("--count", type=int, default=2000)
    parser.add_argument("--jobs", type=int, default=os.cpu_count() or 1, help="judgments checked concurrently")
    parser.add_argument("--timeout", type=float, default=TIMEOUT, help="seconds one command may run")
    args = parser.parse_args()
    TIMEOUT = args.timeout
    if not os.environ.get("ONTOS_PY_ROOT"):
        raise SystemExit("Set ONTOS_PY_ROOT to an ontos v0.17.0 checkout: the Python CLI is one of the cores")
    if args.jobs <= 0:
        raise SystemExit("jobs must be positive")
    if args.seed is None:
        checks, findings, slowest = run_units(list(oracle_units()), args.jobs)
    else:
        if args.count <= 0:
            raise SystemExit("count must be positive")
        generated = subprocess.run(
            [str(ROOT / "target/release/horos-fuzz"), "--seed", str(args.seed), "--count", str(args.count)],
            capture_output=True,
            text=True,
            timeout=300,
            check=True,
        )
        lines = generated.stdout.splitlines()
        if len(lines) != args.count:
            raise AssertionError("generator count mismatch")
        if generated.stderr.strip():
            print(generated.stderr.strip(), flush=True)
        checks, findings, slowest = run_units([pair_unit(*line.split()) for line in lines], args.jobs)
    for finding in findings:
        print(json.dumps({"seed": args.seed, "finding": finding}, sort_keys=True), flush=True)
    if findings:
        raise SystemExit(f"{len(findings)} disagreements; seed={args.seed}")
    print(f"{checks} judgments agree across {len(CORES)} cores; seed={args.seed}; slowest unit {slowest:.1f}s", flush=True)


if __name__ == "__main__":
    main()
