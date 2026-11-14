"""Time the four checkers on ordinary input and print the medians (ADR 0013).

Each core's bench builds a list of n closed records {name text, age opt int, tags list-of
text}, with age's type an int ("plain") or a three-way union of leaves ("union"), and
times conforms and diagnose on it: core/go/internal/bench, core/rs/examples/bench.rs,
core/ts/bench/bench.mjs and core/py/bench.py. No check runs this; it measures the cost
of checking, which seeded conformance does not. `--against <checkout>` builds a second
checkout's benches too (copy these bench files into it first if it predates them) and
runs the two back to back, in alternating order (base then this, then this then base),
so changing load on a shared machine does not favour either side. It prints both
medians and the change: the median of the per-run ratios, this over base.

On a machine shared with other work, a memory-bound bench's time also depends on where
each process lands: which CPU, which L3 cache, how busy its SMT sibling is. One process
can take twice as long as the next on the same code. `--pin CPU` runs every bench on
that one CPU, so both checkouts meet the same conditions.

Only the cores named by `--cores` are built and timed. A checkout whose core/ts has no
node_modules gets them from `npm ci` before its TypeScript bench is built.

Allocation is reported as each runtime allows: Go's and Rust's bytes allocated, the heap
growth V8 reports for TypeScript, and the peak traced memory for Python (a separate,
slower run).

Run:  ONTOS_PY_ROOT=<ontos v0.17.0 checkout> python3 scripts/bench.py [--n 200000] [--runs 3] [--against DIR]
      [--cores go,rust,typescript,python] [--pin CPU]
ONTOS_PY_ROOT is needed only when the Python core is timed.
"""

from __future__ import annotations

import argparse
import json
import os
import shutil
import statistics
import subprocess
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
CORES = ("go", "rust", "typescript", "python")


def build(root: Path, tmp: Path, cores: list) -> dict:
    """Build root's benches for the given cores into tmp; return each one's command."""
    commands = {}
    if "go" in cores:
        go = tmp / "bench-go"
        subprocess.run(["go", "build", "-o", str(go), "./internal/bench"], cwd=root / "core/go", check=True)
        commands["go"] = [str(go)]
    if "rust" in cores:
        target = tmp / "target"
        env = {**os.environ, "CARGO_TARGET_DIR": str(target)}
        subprocess.run(["cargo", "build", "--locked", "--release", "--example", "bench", "--quiet"], cwd=root, env=env, check=True)
        commands["rust"] = [str(target / "release/examples/bench")]
    if "typescript" in cores:
        ts = root / "core/ts"
        if not (ts / "node_modules").is_dir():
            subprocess.run(["npm", "ci", "--silent"], cwd=ts, check=True)
        subprocess.run(["npm", "run", "build", "--silent"], cwd=ts, check=True)
        commands["typescript"] = ["node", "--expose-gc", str(ts / "bench/bench.mjs")]
    if "python" in cores:
        commands["python"] = ["python3", "-I", str(root / "core/py/bench.py")]
    return commands


def run(command: list, n: int, mode: str, extra: list = (), pin: str | None = None) -> dict:
    prefix = ["taskset", "-c", pin] if pin is not None else []
    out = subprocess.run(prefix + command + [str(n), mode, *extra], capture_output=True, text=True, check=True)
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
    parser.add_argument("--cores", default=",".join(CORES), help="comma-separated cores to time")
    parser.add_argument("--pin", metavar="CPU", help="run every bench on this CPU (taskset -c), for a shared machine")
    args = parser.parse_args()
    cores = args.cores.split(",")
    unknown = [c for c in cores if c not in CORES]
    if unknown:
        raise SystemExit(f"Unknown core {', '.join(unknown)}; the cores are {', '.join(CORES)}")
    if "python" in cores and not os.environ.get("ONTOS_PY_ROOT"):
        raise SystemExit("Set ONTOS_PY_ROOT to an ontos v0.17.0 checkout")
    if args.pin is not None and shutil.which("taskset") is None:
        raise SystemExit("--pin needs taskset (util-linux)")
    trees = {"this": ROOT} if args.against is None else {"base": args.against.resolve(), "this": ROOT}
    with tempfile.TemporaryDirectory() as tmp:
        commands = {name: build(root, Path(tmp) / name, cores) for name, root in trees.items()}
        rows = []
        for mode in ("plain", "union"):
            for core in cores:
                samples = {name: [] for name in trees}
                for i in range(args.runs):
                    for name in (list(trees) if i % 2 == 0 else list(trees)[::-1]):
                        samples[name].append(run(commands[name][core], args.n, mode, pin=args.pin))
                change = {}
                if args.against is not None:
                    for op in ("conforms", "diagnose"):
                        ratios = [t[f"{op}_ms"] / b[f"{op}_ms"] for b, t in zip(samples["base"], samples["this"]) if b.get(f"{op}_ms")]
                        if ratios:
                            change[op] = statistics.median(ratios) - 1
                if core == "python":
                    for name in trees:
                        samples[name][0].update(run(commands[name][core], args.n, mode, ["--memory"], pin=args.pin))
                row = {name: medians(samples[name]) for name in trees}
                rows.append((core, mode, row, change))
                print(json.dumps({"core": core, "mode": mode, "n": args.n, "runs": args.runs, **row, "change": change}), flush=True)
    print()
    print(f"n = {args.n}, median of {args.runs} runs" + (f", alternating with {args.against}" if args.against else "")
          + (f", pinned to CPU {args.pin}" if args.pin is not None else ""))
    print()
    if args.against is None:
        print("| core | mode | conforms ms | conforms memory | diagnose ms | diagnose memory |")
        print("| --- | --- | --- | --- | --- | --- |")
        for core, mode, row, _ in rows:
            m = row["this"]
            print(f"| {core} | {mode} | {m.get('conforms_ms', 0):.0f} | {memory(m, 'conforms')} | {m.get('diagnose_ms', 0):.0f} | {memory(m, 'diagnose')} |")
        return
    print("The change in brackets is the median of the per-run ratios, this over base.")
    print()
    print("| core | mode | conforms ms (base → this) | conforms memory | diagnose ms (base → this) | diagnose memory |")
    print("| --- | --- | --- | --- | --- | --- |")
    for core, mode, row, change in rows:
        b, t = row["base"], row["this"]

        def ms(op):
            x, y = b.get(f"{op}_ms", 0), t.get(f"{op}_ms", 0)
            return f"{x:.0f} → {y:.0f} ({change[op] * 100:+.0f}%)" if op in change else ""

        def mem(op):
            return f"{memory(b, op)} → {memory(t, op)}"

        print(f"| {core} | {mode} | {ms('conforms')} | {mem('conforms')} | {ms('diagnose')} | {mem('diagnose')} |")


if __name__ == "__main__":
    main()
