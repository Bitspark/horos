"""Source of vectors/integer-bounds.json, the hand-authored oracle for int-min and int-max.

The original oracle, vectors/conformance.json, is pinned byte for byte, so the
judgments for the constructors ADR 0010 adds live here, in the same format. Each
expected `wf` and `conforms` below is written from docs/spec/horos.md and ADR 0010,
never from checker output. This script only encodes the inputs as ontos-codec-v1
hex. Values include integers beyond 128 bits on both sides.

Run:  ONTOS_PY_ROOT=<ontos v0.17.0 checkout> python3 vectors/integer-bounds.py [--check]
"""

from __future__ import annotations

import json
import os
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
OUT = ROOT / "vectors" / "integer-bounds.json"
COPIES = [ROOT / "core" / "go" / "testdata" / "integer-bounds.json", ROOT / "core" / "rs" / "testdata" / "integer-bounds.json"]

_ONTOS = os.environ.get("ONTOS_PY_ROOT")
if not _ONTOS:
    raise SystemExit("Set ONTOS_PY_ROOT to an ontos v0.17.0 checkout")
for sub in ("core/py", "codec/py", "data/py"):
    sys.path.insert(0, str(Path(_ONTOS) / sub))

from ontos_codec import encode  # noqa: E402
from ontos_core import atom, tuple_  # noqa: E402
from ontos_data import encode_bool, encode_int, encode_list, encode_map, encode_text  # noqa: E402


def A(s):
    return atom(s.encode("utf-8"))


def I(n):  # noqa: E743
    return encode_int(n)


def T(s):
    return encode_text(s)


def d(ctor, *args):
    return tuple_([A("horos/" + ctor), *args])


def field(name, presence, ty):
    return d("field", T(name), A(presence), ty)


I128_MAX = 2**127 - 1
I128_MIN = -(2**127)
WIDE = 2**200
# Not canonical ints: a leading zero magnitude byte, and negative zero.
LEADING_ZERO = tuple_([A("int"), atom(bytes([0x00, 0x00, 0x01]))])
NEGATIVE_ZERO = tuple_([A("int"), atom(bytes([0x01]))])

CASES = []


def case(name, descriptor, wf, *values):
    CASES.append(
        {
            "name": name,
            "descriptor": encode(descriptor).hex(),
            "wf": wf,
            "values": [{"note": note, "value": encode(v).hex(), "conforms": c} for note, v, c in values],
        }
    )


case(
    "int-min-0",
    d("int-min", I(0)),
    True,
    ("the bound itself", I(0), True),
    ("above", I(1), True),
    ("below", I(-1), False),
    ("2^200, beyond 128 bits", I(WIDE), True),
    ("-2^200, beyond 128 bits", I(-WIDE), False),
    ("an atom", A("5"), False),
    ("text", T("5"), False),
    ("a bool", encode_bool(True), False),
    ("a non-canonical leading zero", LEADING_ZERO, False),
    ("a non-canonical negative zero", NEGATIVE_ZERO, False),
)
case(
    "int-max-9",
    d("int-max", I(9)),
    True,
    ("the bound itself", I(9), True),
    ("above", I(10), False),
    ("a negative", I(-9), True),
    ("-2^200", I(-WIDE), True),
    ("2^200", I(WIDE), False),
    ("an atom", A("9"), False),
)
case(
    "int-min-negative",
    d("int-min", I(-10)),
    True,
    ("the bound itself", I(-10), True),
    ("below", I(-11), False),
    ("zero", I(0), True),
)
case(
    "int-min-at-i128-max",
    d("int-min", I(I128_MAX)),
    True,
    ("the bound itself", I(I128_MAX), True),
    ("2^127, just past i128", I(I128_MAX + 1), True),
    ("one below", I(I128_MAX - 1), False),
    ("2^200", I(WIDE), True),
    ("-2^127 - 1, just past i128", I(I128_MIN - 1), False),
)
case(
    "int-max-at-i128-min",
    d("int-max", I(I128_MIN)),
    True,
    ("the bound itself", I(I128_MIN), True),
    ("-2^127 - 1, just past i128", I(I128_MIN - 1), True),
    ("one above", I(I128_MIN + 1), False),
    ("-2^200", I(-WIDE), True),
    ("zero", I(0), False),
)
case(
    "and-closed-range",
    d("and", d("int-min", I(0)), d("int-max", I(9))),
    True,
    ("low end", I(0), True),
    ("high end", I(9), True),
    ("above", I(10), False),
    ("below", I(-1), False),
    ("2^200", I(WIDE), False),
    ("-2^200", I(-WIDE), False),
)
case(
    "and-empty-range",
    d("and", d("int-min", I(5)), d("int-max", I(1))),
    True,
    ("between the bounds", I(3), False),
    ("at the lower bound", I(5), False),
    ("at the upper bound", I(1), False),
)
case(
    "not-int-min-0",
    d("not", d("int-min", I(0))),
    True,
    ("a negative", I(-1), True),
    ("zero", I(0), False),
    ("not an int at all", A("x"), True),
)
case(
    "record-count-non-negative",
    d("record", A("closed"), field("count", "req", d("int-min", I(0)))),
    True,
    ("count 3", encode_map([(T("count"), I(3))]), True),
    ("count -1", encode_map([(T("count"), I(-1))]), False),
    ("count 2^200", encode_map([(T("count"), I(WIDE))]), True),
)
case(
    "list-of-int-max-255",
    d("list-of", d("int-max", I(255))),
    True,
    ("in range, with -2^200", encode_list([I(255), I(-WIDE)]), True),
    ("2^200 is above", encode_list([I(1), I(WIDE)]), False),
)
case("wf-int-min-bound-2pow127", d("int-min", I(I128_MAX + 1)), False, ("an int", I(0), False))
case("wf-int-max-bound-below-i128", d("int-max", I(I128_MIN - 1)), False, ("an int", I(0), False))
case("wf-int-min-bound-2pow200", d("int-min", I(WIDE)), False, ("2^200", I(WIDE), False))
case("wf-int-min-no-bound", d("int-min"), False, ("an int", I(0), False))
case("wf-int-max-two-bounds", d("int-max", I(1), I(2)), False, ("an int", I(0), False))
case("wf-int-min-atom-bound", d("int-min", A("0")), False, ("an int", I(0), False))
case("wf-int-max-text-bound", d("int-max", T("9")), False, ("an int", I(0), False))
case("wf-int-min-non-canonical-bound", d("int-min", LEADING_ZERO), False, ("an int", I(1), False))
case("wf-and-malformed-bound", d("and", d("int-min", I(0)), d("int-max", NEGATIVE_ZERO)), False, ("an int", I(0), False))


def render():
    doc = {
        "version": "horos-integer-bounds-v1",
        "note": (
            "Hand-authored oracle for int-min and int-max (ADR 0010). `descriptor` and `value` "
            "are ontos-codec-v1 lowercase hex; `wf`/`conforms` are the expected judgments, "
            "authored from docs/spec/horos.md. Generated inputs only: vectors/integer-bounds.py "
            "encodes them."
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
            raise SystemExit(f"stale integer-bounds oracle: {', '.join(stale)}; run vectors/integer-bounds.py")
        print(f"integer-bounds oracle current: {len(CASES)} cases")
        return
    for p in targets:
        p.write_text(text, encoding="utf-8")
    print(f"wrote {len(CASES)} cases")


if __name__ == "__main__":
    main()
