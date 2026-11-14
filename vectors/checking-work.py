"""Source of vectors/checking-work.json, the hand-authored oracle for checking work (ADR 0013).

Each descriptor below has alternatives that overlap, so a value can be reached along
many routes; checked route by route, the work doubles with every level of the value.
The expected `wf`, `conforms`, `diagnose_wf` and `diagnosis` are written from
docs/spec/horos.md and ADRs 0008, 0009 and 0013, never from checker output; this script
only encodes the inputs. The values are 64 levels deep, so a checker that is not bounded
by the size of its input would not finish them; one that is finishes them at once.

Run:  ONTOS_PY_ROOT=<ontos v0.17.0 checkout> python3 vectors/checking-work.py [--check]
"""

from __future__ import annotations

import json
import os
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
OUT = ROOT / "vectors" / "checking-work.json"
COPIES = [ROOT / "core" / "go" / "testdata" / "checking-work.json", ROOT / "core" / "rs" / "testdata" / "checking-work.json"]

_ONTOS = os.environ.get("ONTOS_PY_ROOT")
if not _ONTOS:
    raise SystemExit("Set ONTOS_PY_ROOT to an ontos v0.17.0 checkout")
for sub in ("core/py", "codec/py", "data/py"):
    sys.path.insert(0, str(Path(_ONTOS) / sub))

from ontos_codec import encode  # noqa: E402
from ontos_core import atom, tuple_  # noqa: E402
from ontos_data import encode_list  # noqa: E402


def A(s):
    return atom(s.encode("utf-8"))


def d(ctor, *args):
    return tuple_([A("horos/" + ctor), *args])


def elem(i):
    return {"kind": "elem", "index": i}


OK = {"verdict": "conforms"}


def MIS(path, at, reason):
    return {"verdict": "mismatch", "location": {"path": path, "at": at, "reason": reason}}


CASES = []


def case(name, descriptor, *values):
    for note, _, conforms, diagnosis in values:
        assert conforms == (diagnosis == OK), (name, note)
    CASES.append(
        {
            "name": name,
            "descriptor": encode(descriptor).hex(),
            "wf": True,
            "diagnose_wf": OK,
            "values": [
                {"note": note, "value": encode(v).hex(), "conforms": conforms, "diagnosis": diagnosis}
                for note, v, conforms, diagnosis in values
            ],
        }
    )


def nested(n, inner):
    """`n` nested one-element lists around `inner`."""
    v = inner
    for _ in range(n):
        v = encode_list([v])
    return v


EMPTY = encode_list([])
X = A("x")
DEPTH = 64

# L = list of L | list of L: both alternatives accept every list. Positions: the union is
# [2], its alternatives [2, 1] and [2, 2]; each element's ref(L) continues at [2]. Level k
# enters the union at depth 1 + 3k, so 64 levels end at 193, well inside the bound.
UNION = d("rec", A("L"), d("union", d("list-of", d("ref", A("L"))), d("list-of", d("ref", A("L")))))
case(
    "overlapping-union",
    UNION,
    ("64 nested lists around the empty list", nested(DEPTH, EMPTY), True, OK),
    # The innermost x is refused by both alternatives at the same value, so its union
    # names itself; every union above passes that deeper location on.
    (
        "64 nested lists around an atom: the innermost union has no alternative",
        nested(DEPTH, X),
        False,
        MIS([elem(0)] * DEPTH, [2], "no-alternative"),
    ),
)

# L = list of L and list of L: a conforming value must pass both conjuncts at every level.
# Positions: the intersection is [2], its conjuncts [2, 1] and [2, 2].
AND = d("rec", A("L"), d("and", d("list-of", d("ref", A("L"))), d("list-of", d("ref", A("L")))))
case(
    "overlapping-intersection",
    AND,
    ("64 nested lists around the empty list", nested(DEPTH, EMPTY), True, OK),
    # The first conjunct mismatches first, at every level, down to x refused by the
    # innermost first conjunct's list-of.
    (
        "64 nested lists around an atom: the first conjunct refuses it",
        nested(DEPTH, X),
        False,
        MIS([elem(0)] * DEPTH, [2, 1], "refused"),
    ),
)


def render():
    doc = {
        "version": "horos-checking-work-v1",
        "note": (
            "Hand-authored oracle for checking work (ADR 0013): overlapping alternatives over "
            "deep values, which a checker bounded by its input finishes at once. `descriptor` "
            "and `value` are ontos-codec-v1 lowercase hex; `wf`/`conforms` are the expected "
            "booleans and `diagnose_wf`/`diagnosis` the expected diagnoses, authored from the "
            "specification. Generated inputs only: vectors/checking-work.py encodes them."
        ),
        "cases": CASES,
    }
    return json.dumps(doc, indent=2, sort_keys=True, ensure_ascii=False) + "\n"


def main():
    text = render()
    targets = [OUT, *COPIES]
    if "--check" in sys.argv[1:]:
        stale = [str(p.relative_to(ROOT)) for p in targets if not p.is_file() or p.read_text(encoding="utf-8") != text]
        if stale:
            raise SystemExit(f"stale checking-work oracle: {', '.join(stale)}; run vectors/checking-work.py")
        print(f"checking-work oracle current: {len(CASES)} cases")
        return
    for p in targets:
        p.write_text(text, encoding="utf-8")
    print(f"wrote {len(CASES)} cases")


if __name__ == "__main__":
    main()
