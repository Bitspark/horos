"""Time the four checkers on ordinary input and print the medians (ADR 0013).

Each core's bench builds a list of n closed records {name text, age opt int, tags list-of
text}, with age's type an int ("plain") or a three-way union of leaves ("union"), and
times conforms and diagnose on it: core/go/internal/bench, core/rs/examples/bench.rs,
core/ts/bench/bench.mjs and core/py/bench.py. No check runs this; it measures the cost
of checking, which seeded conformance does not. `--against <checkout>` builds a second
checkout's benches too (copy these bench files into it first if it predates them) and
alternates the two run by run, so changing load on a shared machine does not favour
either side; it prints both medians and their ratio.

Allocation is reported as each runtime allows: Go's and Rust's bytes allocated, the heap
growth V8 reports for TypeScript, and the peak traced memory for Python (a separate,
slower run).

Run:  ONTOS_PY_ROOT=<ontos v0.17.0 checkout> python3 scripts/bench.py [--n 200000] [--runs 3] [--against DIR]
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


def build(root: Path, tmp: Path) -> dict:
    """Build root's benches into tmp; return each core's command."""
    target = tmp / "target"
    go = tmp / "bench-go"
    subprocess.run(["go", "build", "-o", str(go), "./internal/bench"], cwd=root / "core/go", check=True)
    env = {**os.environ, "CARGO_TARGET_DIR": str(target)}
    subprocess.run(["cargo", "build", "--locked", "--release", "--example", "bench", "--quiet"], cwd=root, env=env, check=True)
    subprocess.run(["npm", "run", "build", "--silent"], cwd=root / "core/ts", check=True)
    return {
        "go": [str(go)],
        "rust": [str(target / "release/examples/bench")],
        "typescript": ["node", "--expose-gc", str(root / "core/ts/bench/bench.mjs")],
        "python": ["python3", "-I", str(root / "core/py/bench.py")],
    }


def run(command: list, n: int, mode: str, extra: list = ()) -> dict:
    out = subprocess.run(command + [str(n), mode, *extra], capture_output=True, text=True, check=True)
    return json.loads(out.stdout)


def medians(samples: list) -> dict:
    keys = sorted({k for s in samples for k, v in s.items() if isinstance(v, (int, float)) and not isinstance(v, bool) and k != "n"})
    return {k: statistics.median(s[k] for s in samples if k in s) for k in keys}


def memory(m: dict, op: str) -> str:
    for key, unit in ((f"{op}_mb", "MB"), (f"{op}_heap_mb", "MB heap"), (f"{op}_peak_mb", "MB peak")):
        if key in m:
            return f"{m[key]:.1f} {unit}"
    return ""


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--n", type=int, default=200_000)
    parser.add_argument("--runs", type=int, default=3)
    parser.add_argument("--against", type=Path, help="a second checkout, run alternately with this one")
    args = parser.parse_args()
    if not os.environ.get("ONTOS_PY_ROOT"):
        raise SystemExit("Set ONTOS_PY_ROOT to an ontos v0.17.0 checkout")
    trees = {"this": ROOT} if args.against is None else {"base": args.against.resolve(), "this": ROOT}
    with tempfile.TemporaryDirectory() as tmp:
        cores = {name: build(root, Path(tmp) / name) for name, root in trees.items()}
        rows = []
        for mode in ("plain", "union"):
            for core in ("go", "rust", "typescript", "python"):
                samples = {name: [] for name in trees}
                for _ in range(args.runs):
                    for name in trees:
                        samples[name].append(run(cores[name][core], args.n, mode))
                if core == "python":
                    for name in trees:
                        samples[name][0].update(run(cores[name][core], args.n, mode, ["--memory"]))
                row = {name: medians(samples[name]) for name in trees}
                rows.append((core, mode, row))
                print(json.dumps({"core": core, "mode": mode, "n": args.n, "runs": args.runs, **row}), flush=True)
    print()
    print(f"n = {args.n}, median of {args.runs} runs" + (f", alternating with {args.against}" if args.against else ""))
    print()
    if args.against is None:
        print("| core | mode | conforms ms | conforms memory | diagnose ms | diagnose memory |")
        print("| --- | --- | --- | --- | --- | --- |")
        for core, mode, row in rows:
            m = row["this"]
            print(f"| {core} | {mode} | {m.get('conforms_ms', 0):.0f} | {memory(m, 'conforms')} | {m.get('diagnose_ms', 0):.0f} | {memory(m, 'diagnose')} |")
        return
    print("| core | mode | conforms ms (base → this) | conforms memory | diagnose ms (base → this) | diagnose memory |")
    print("| --- | --- | --- | --- | --- | --- |")
    for core, mode, row in rows:
        b, t = row["base"], row["this"]

        def ms(op):
            x, y = b.get(f"{op}_ms", 0), t.get(f"{op}_ms", 0)
            return f"{x:.0f} → {y:.0f} ({(y / x - 1) * 100:+.0f}%)" if x else ""

        def mem(op):
            return f"{memory(b, op)} → {memory(t, op)}"

        print(f"| {core} | {mode} | {ms('conforms')} | {mem('conforms')} | {ms('diagnose')} | {mem('diagnose')} |")


if __name__ == "__main__":
    main()
