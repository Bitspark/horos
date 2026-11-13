"""Time the four checkers on ordinary input and print the medians (ADR 0013).

Each core's bench builds a list of n closed records {name text, age opt int, tags list-of
text}, with age's type an int ("plain") or a three-way union of leaves ("union"), and
times conforms and diagnose on it: core/go/internal/bench, core/rs/examples/bench.rs,
core/ts/bench/bench.mjs and core/py/bench.py. No check runs this; it measures the cost
of checking, which seeded conformance does not. Run it on two checkouts to compare them.

Allocation is reported as each runtime allows: Go's and Rust's bytes allocated, the heap
growth V8 reports for TypeScript, and the peak traced memory for Python (a separate,
slower run).

Run:  ONTOS_PY_ROOT=<ontos v0.17.0 checkout> python3 scripts/bench.py [--n 200000] [--runs 3]
"""

from __future__ import annotations

import argparse
import json
import os
import statistics
import subprocess
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent


def build(tmp: Path) -> dict:
    go = tmp / "bench-go"
    subprocess.run(["go", "build", "-o", str(go), "./internal/bench"], cwd=ROOT / "core/go", check=True)
    subprocess.run(["cargo", "build", "--locked", "--release", "--example", "bench", "--quiet"], cwd=ROOT, check=True)
    subprocess.run(["npm", "run", "build", "--silent"], cwd=ROOT / "core/ts", check=True)
    return {
        "go": [str(go)],
        "rust": [str(Path(os.environ.get("CARGO_TARGET_DIR", ROOT / "target")) / "release/examples/bench")],
        "typescript": ["node", "--expose-gc", str(ROOT / "core/ts/bench/bench.mjs")],
        "python": ["python3", "-I", str(ROOT / "core/py/bench.py")],
    }


def run(command: list, n: int, mode: str, extra: list = ()) -> dict:
    out = subprocess.run(command + [str(n), mode, *extra], capture_output=True, text=True, check=True)
    return json.loads(out.stdout)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--n", type=int, default=200_000)
    parser.add_argument("--runs", type=int, default=3)
    args = parser.parse_args()
    if not os.environ.get("ONTOS_PY_ROOT"):
        raise SystemExit("Set ONTOS_PY_ROOT to an ontos v0.17.0 checkout")
    with tempfile.TemporaryDirectory() as tmp:
        cores = build(Path(tmp))
        rows = []
        for mode in ("plain", "union"):
            for core, command in cores.items():
                samples = [run(command, args.n, mode) for _ in range(args.runs)]
                if core == "python":
                    samples[0].update(run(command, args.n, mode, ["--memory"]))
                metrics = sorted({k for s in samples for k, v in s.items() if isinstance(v, (int, float)) and not isinstance(v, bool) and k != "n"})
                medians = {k: statistics.median(s[k] for s in samples if k in s) for k in metrics}
                verdicts = {(s["conforms"], s["diagnose"]) for s in samples}
                rows.append((core, mode, verdicts, medians))
                print(json.dumps({"core": core, "mode": mode, "n": args.n, "runs": args.runs, **medians}), flush=True)
    print()
    print(f"| core | mode | conforms ms | conforms memory | diagnose ms | diagnose memory |  (n = {args.n}, median of {args.runs})")
    print("| --- | --- | --- | --- | --- | --- |")
    for core, mode, verdicts, m in rows:
        mem = lambda op: next((f"{m[k]:.1f} {u}" for k, u in ((f"{op}_mb", "MB"), (f"{op}_heap_mb", "MB heap"), (f"{op}_peak_mb", "MB peak")) if k in m), "")  # noqa: E731
        print(f"| {core} | {mode} | {m.get('conforms_ms', 0):.0f} | {mem('conforms')} | {m.get('diagnose_ms', 0):.0f} | {mem('diagnose')} |")


if __name__ == "__main__":
    main()
