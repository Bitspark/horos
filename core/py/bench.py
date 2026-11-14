"""Time conforms and diagnose on ordinary input: a list of n closed records
{name text, age opt int, tags list-of text}, with age's type an int ("plain") or a
three-way union of leaves ("union"). It measures the checker, not seeded conformance,
and no check runs it; scripts/bench.py runs it beside the other cores (ADR 0013).
`--memory` reports the peak traced memory of each call instead, in a slower run.

Run:  ONTOS_PY_ROOT=<ontos v0.17.0 checkout> python3 -I core/py/bench.py 200000 plain [--memory]
"""

from __future__ import annotations

import json
import os
import sys
import time
import tracemalloc
from pathlib import Path


def main() -> None:
    root = os.environ.get("ONTOS_PY_ROOT")
    if not root:
        raise SystemExit("Set ONTOS_PY_ROOT to an ontos v0.17.0 checkout")
    for sub in ("core/py", "codec/py", "data/py"):
        sys.path.insert(0, str(Path(root) / sub))
    sys.path.insert(0, str(Path(__file__).resolve().parent))
    from ontos_core import atom, tuple_
    from ontos_data import encode_int, encode_list, encode_map, encode_null, encode_text

    from horos import conforms
    from horos.diagnose import diagnose

    args = [a for a in sys.argv[1:] if a != "--memory"]
    if len(args) != 2 or args[1] not in ("plain", "union"):
        raise SystemExit("usage: bench.py <n> plain|union [--memory]")
    n, mode = int(args[0]), args[1]
    memory = "--memory" in sys.argv[1:]

    def a(s):
        return atom(s.encode("utf-8"))

    def d(ctor, *rest):
        return tuple_([a("horos/" + ctor), *rest])

    def field(name, presence, ty):
        return d("field", encode_text(name), a(presence), ty)

    age = d("scalar", a("int"))
    if mode == "union":
        age = d("union", d("exact", encode_null()), d("scalar", a("int")), d("scalar", a("utf8-text")))
    desc = d(
        "list-of",
        d(
            "record",
            a("closed"),
            field("name", "req", d("scalar", a("utf8-text"))),
            field("age", "opt", age),
            field("tags", "req", d("list-of", d("scalar", a("utf8-text")))),
        ),
    )
    v = encode_list(
        [
            encode_map(
                [
                    (encode_text("age"), encode_int(i)),
                    (encode_text("name"), encode_text(f"n{i}")),
                    (encode_text("tags"), encode_list([encode_text("a"), encode_text("b"), encode_text("c")])),
                ]
            )
            for i in range(n)
        ]
    )

    def measure(run):
        if memory:
            tracemalloc.start()
        start = time.perf_counter()
        out = run()
        ms = round((time.perf_counter() - start) * 1000)
        peak = 0.0
        if memory:
            peak = tracemalloc.get_traced_memory()[1] / 1e6
            tracemalloc.stop()
        return out, ms, peak

    ok, cms, cpeak = measure(lambda: conforms(v, desc))
    dg, dms, dpeak = measure(lambda: diagnose(v, desc).verdict)
    out = {"core": "python", "n": n, "mode": mode, "conforms": ok, "diagnose": dg}
    if memory:
        out.update(conforms_peak_mb=round(cpeak, 1), diagnose_peak_mb=round(dpeak, 1))
    else:
        out.update(conforms_ms=cms, diagnose_ms=dms)
    print(json.dumps(out))


if __name__ == "__main__":
    main()
