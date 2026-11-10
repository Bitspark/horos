"""Source of vectors/diagnose.json, the hand-authored diagnostic-location corpus.

Each expected diagnosis below is written from docs/spec/horos.md#diagnostic-locations
and ADR 0009, never from checker output. This script only encodes the authored
descriptors and values as ontos-codec-v1 hex. Maps and sets are written in their
canonical order, so authored indices are the canonical ones; ontos's own readers
confirm that order instead of silently re-sorting.

Run:  ONTOS_PY_ROOT=<ontos v0.16.0 checkout> python3 vectors/diagnose.py [--check]
"""

from __future__ import annotations

import json
import os
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
OUT = ROOT / "vectors" / "diagnose.json"
COPIES = [ROOT / "core" / "go" / "testdata" / "diagnose.json", ROOT / "core" / "rs" / "testdata" / "diagnose.json"]

_ONTOS = os.environ.get("ONTOS_PY_ROOT")
if not _ONTOS:
    raise SystemExit("Set ONTOS_PY_ROOT to an ontos v0.16.0 checkout")
for sub in ("core/py", "codec/py", "data/py"):
    sys.path.insert(0, str(Path(_ONTOS) / sub))

from ontos_codec import encode  # noqa: E402
from ontos_core import atom, tuple_  # noqa: E402
from ontos_data import encode_bool, encode_int, encode_list, encode_text, read_map, read_set  # noqa: E402

# ----- authored inputs ---------------------------------------------------------


def A(s):
    return atom(s.encode("utf-8"))


def I(n):  # noqa: E743
    return encode_int(n)


def T(s):
    return encode_text(s)


def B(b):
    return encode_bool(b)


def L(*elements):
    return encode_list(list(elements))


def M(*entries):
    """A map written in canonical key order; ontos's reader rejects any other order."""
    v = tuple_([A("map"), *(tuple_([k, val]) for k, val in entries)])
    read_map(v)
    return v


def S(*elements):
    """A set written in canonical order; ontos's reader rejects any other order."""
    v = tuple_([A("set"), *elements])
    read_set(v)
    return v


def Tup(*items):
    return tuple_(list(items))


def d(ctor, *args):
    return tuple_([A("horos/" + ctor), *args])


def field(name, presence, ty):
    return d("field", T(name), A(presence), ty)


def nest(ctor, n, inner):
    for _ in range(n):
        inner = d(ctor, inner)
    return inner


ANY = d("any")
INT = d("scalar", A("int"))
TEXT = d("scalar", A("utf8-text"))
BOOL = d("scalar", A("bool"))

# ----- authored expectations ---------------------------------------------------


def item(i):
    return {"kind": "item", "index": i}


def elem(i):
    return {"kind": "elem", "index": i}


def key(i):
    return {"kind": "key", "index": i}


def val(i):
    return {"kind": "value", "index": i}


def fld(name):
    return {"kind": "field", "name": name}


def loc(path, at, reason, field=None, arg=None):
    out = {"path": path, "at": at, "reason": reason}
    if field is not None:
        out["field"] = field
    if arg is not None:
        out["arg"] = arg
    return out


OK = {"verdict": "conforms"}


def MIS(*a, **k):
    return {"verdict": "mismatch", "location": loc(*a, **k)}


def NWF(*a, **k):
    return {"verdict": "not_well_formed", "location": loc(*a, **k)}


def EXH(*a, **k):
    return {"verdict": "exhausted", "limit": "recursion_depth", "location": loc(*a, **k)}


CASES = []


def case(name, descriptor, wf, *values):
    CASES.append(
        {
            "name": name,
            "descriptor": encode(descriptor).hex(),
            "wf": wf,
            "values": [{"note": note, "value": encode(v).hex(), "diagnosis": diag} for note, v, diag in values],
        }
    )


def malformed(name, descriptor, location):
    """A malformed descriptor: diagnoseWf and diagnose report the same location."""
    diag = {"verdict": "not_well_formed", "location": location}
    case(name, descriptor, diag, ("any value", A("x"), diag))


# Records. Field k's type sits at [k, 3]: item k of the record, item 3 of the field form.
PERSON = d("record", A("closed"), field("name", "req", TEXT), field("age", "opt", INT))
case(
    "record-closed-fields",
    PERSON,
    OK,
    ("name present", M((T("name"), T("Ada"))), OK),
    ("required name missing", M(), MIS([], [], "missing-field", field="name")),
    ("extra key in a closed record", M((T("zip"), I(1)), (T("name"), T("Ada"))), MIS([], [], "extra-field", field="zip")),
    ("missing before extra", M((T("zip"), I(1))), MIS([], [], "missing-field", field="name")),
    ("name is not text", M((T("name"), I(5))), MIS([fld("name")], [2, 3], "refused")),
    ("missing before a bad present field", M((T("age"), T("x"))), MIS([], [], "missing-field", field="name")),
    ("children in descriptor order", M((T("age"), T("x")), (T("name"), T("Ada"))), MIS([fld("age")], [3, 3], "refused")),
    ("not a map", A("x"), MIS([], [], "refused")),
    ("first non-text key", M((I(1), T("x")), (T("name"), T("Ada"))), MIS([key(0)], [], "refused")),
)

ITEMS = d(
    "record",
    A("open"),
    field("items", "req", d("list-of", d("record", A("closed"), field("id", "req", INT)))),
)
case(
    "record-nested-list",
    ITEMS,
    OK,
    ("open record ignores an extra key", M((T("extra"), B(True)), (T("items"), L(M((T("id"), I(1)))))), OK),
    (
        "second element's id is not an int",
        M((T("extra"), B(True)), (T("items"), L(M((T("id"), I(1))), M((T("id"), T("x")))))),
        MIS([fld("items"), elem(1), fld("id")], [2, 3, 1, 2, 3], "refused"),
    ),
    ("inner closed record has an extra key", M((T("items"), L(M((T("x"), I(2)), (T("id"), I(1)))))), MIS([fld("items"), elem(0)], [2, 3, 1], "extra-field", field="x")),
    ("inner record misses id", M((T("items"), L(M()))), MIS([fld("items"), elem(0)], [2, 3, 1], "missing-field", field="id")),
    ("items is not a list", M((T("items"), T("nope"))), MIS([fld("items")], [2, 3], "refused")),
)

# Maps: entries in canonical order, each key before its value.
case(
    "map-key-versus-value",
    d("map-of", TEXT, INT),
    OK,
    ("empty map", M(), OK),
    ("second value is not an int", M((T("a"), I(1)), (T("b"), T("x"))), MIS([val(1)], [2], "refused")),
    ("key is not text", M((I(1), I(1))), MIS([key(0)], [1], "refused")),
    ("an earlier value before a later key", M((T("a"), T("x")), (Tup(A("a"), A("b"), A("c")), I(1))), MIS([val(0)], [2], "refused")),
    ("not a map", L(), MIS([], [], "refused")),
)

# Bare tuples, sequences, compounds, lists and sets.
case(
    "tuple-items",
    d("tuple", INT, d("atom")),
    OK,
    ("second item is not an atom", Tup(I(1), T("x")), MIS([item(1)], [2], "refused")),
    ("first failing item wins", Tup(A("x"), I(5)), MIS([item(0)], [1], "refused")),
    ("wrong arity", Tup(I(1)), MIS([], [], "refused")),
)
case("seq-items", d("seq", BOOL), OK, ("second item is not a bool", Tup(B(True), I(1)), MIS([item(1)], [1], "refused")))
case(
    "compound-children",
    d("compound", A("pt"), INT, INT),
    OK,
    ("label is item 0, so y is item 2", Tup(A("pt"), I(1), T("x")), MIS([item(2)], [3], "refused")),
    ("different label", Tup(A("pq"), I(1), I(2)), MIS([], [], "refused")),
)
case(
    "list-elements",
    d("list-of", INT),
    OK,
    ("third element is the first failure", L(I(1), I(2), T("x"), T("y")), MIS([elem(2)], [1], "refused")),
)
case(
    "set-elements",
    d("set-of", d("atom-len", I(1), I(1))),
    OK,
    ("canonical order puts the long atom second", S(A("a"), A("bb")), MIS([elem(1)], [1], "refused")),
)
case("exact-literal", d("exact", I(7)), OK, ("different int", I(8), MIS([], [], "refused")), ("same int", I(7), OK))

# Unions report the deepest alternative, else themselves.
case(
    "union-no-alternative",
    d("union", INT, BOOL),
    OK,
    ("no alternative descends", A("x"), MIS([], [], "no-alternative")),
    ("an alternative conforms", B(False), OK),
)
case("union-empty", d("union"), OK, ("bottom", I(1), MIS([], [], "no-alternative")))
case(
    "union-deepest-alternative",
    d("union", d("tuple", INT, INT), d("list-of", INT)),
    OK,
    ("the list alternative descends further", L(I(1), T("x")), MIS([elem(1)], [2, 1], "refused")),
)
case(
    "union-equal-depth-takes-first",
    d("union", d("list-of", INT), d("list-of", BOOL)),
    OK,
    ("both fail at element 0", L(T("x")), MIS([elem(0)], [1, 1], "refused")),
)

# A cons list through rec/ref: the ref continues at the rec body, position [2].
CONS = d("rec", A("L"), d("union", d("exact", A("nil")), d("compound", A("cons"), INT, d("ref", A("L")))))


def cons(*items, tail=None):
    v = A("nil") if tail is None else tail
    for x in reversed(items):
        v = Tup(A("cons"), x, v)
    return v


case(
    "rec-cons-list",
    CONS,
    OK,
    ("empty", cons(), OK),
    ("two ints", cons(I(1), I(2)), OK),
    ("second head is not an int", cons(I(1), T("x")), MIS([item(2), item(1)], [2, 2, 2], "refused")),
    ("bad tail: the union at that depth names itself", cons(I(1), I(2), tail=A("end")), MIS([item(2), item(2)], [2], "no-alternative")),
)

# and, not.
case(
    "and-first-conjunct",
    d("and", d("atom"), d("atom-len", I(1), I(2)), d("not", d("atom-exact", A("no")))),
    OK,
    ("accepted", A("ok"), OK),
    ("too long", A("abc"), MIS([], [2], "refused")),
    ("excluded by the complement", A("no"), MIS([], [3], "complement")),
    ("not an atom: the first conjunct", T("x"), MIS([], [1], "refused")),
)
case("not-complement", d("not", INT), OK, ("an int", I(5), MIS([], [], "complement")), ("an atom", A("x"), OK))
case("not-not", d("not", d("not", INT)), OK, ("an atom", A("x"), MIS([], [], "complement")), ("an int", I(5), OK))
case("list-of-not", d("list-of", d("not", INT)), OK, ("an int element", L(I(1)), MIS([elem(0)], [1], "complement")))

# Exhaustion. Cons level k's union is entered at depth 1 + 3k and its children at
# 3 + 3k, so the 171st cell's children are the first entries past 512.
FLAT_CONS = d("rec", A("L"), d("union", d("exact", A("[]")), d("tuple", INT, d("ref", A("L")))))


def deep_cons(n):
    v = A("[]")
    for _ in range(n):
        v = Tup(I(1), v)
    return v


case(
    "exhausted-cons-list",
    FLAT_CONS,
    OK,
    ("170 cells complete", deep_cons(170), OK),
    ("171 cells reach the bound at the head", deep_cons(171), EXH([item(1)] * 170 + [item(0)], [2, 2, 1], "limit")),
)
# Under one more same-value level, cell 170's alternatives are entered at depth 513.
case(
    "exhausted-union-sibling-mismatch",
    d("union", FLAT_CONS, INT),
    OK,
    ("the exhausted alternative decides", deep_cons(171), EXH([item(1)] * 170, [1, 2, 1], "limit")),
)
case(
    "exhausted-complement",
    d("not", FLAT_CONS),
    OK,
    ("not preserves the inner exhaustion", deep_cons(171), EXH([item(1)] * 170, [1, 2, 1], "limit")),
)
case(
    "and-mismatch-beats-exhausted",
    d("and", FLAT_CONS, INT),
    OK,
    ("the decisive conjunct", deep_cons(171), MIS([], [2], "refused")),
)

# Well-formedness: own form first, then children in item order.
malformed("wf-atom-len-lo-gt-hi", d("atom-len", I(5), I(1)), loc([], [], "argument", arg=2))
malformed("wf-atom-len-negative-lo", d("atom-len", I(-1), I(1)), loc([], [], "argument", arg=1))
malformed("wf-text-len-hi-not-int", d("text-len", I(1), A("x")), loc([], [], "argument", arg=2))
malformed("wf-atom-exact-not-atom", d("atom-exact", I(1)), loc([], [], "argument", arg=1))
malformed("wf-any-arity", d("any", A("x")), loc([], [], "arity"))
malformed("wf-seq-arity", d("seq"), loc([], [], "arity"))
malformed("wf-record-arity", d("record"), loc([], [], "arity"))
malformed("wf-record-mode", d("record", A("sealed")), loc([], [], "argument", arg=1))
malformed("wf-record-field-form", d("record", A("open"), A("x")), loc([], [], "argument", arg=2))
malformed(
    "wf-duplicate-field",
    d("record", A("closed"), field("a", "req", INT), field("a", "opt", BOOL)),
    loc([], [], "duplicate-field", arg=3),
)
malformed(
    "wf-duplicate-before-child",
    d("record", A("closed"), field("a", "req", d("scalar", A("float"))), field("a", "req", INT)),
    loc([], [], "duplicate-field", arg=3),
)
malformed("wf-compound-label", d("compound", T("pt")), loc([], [], "argument", arg=1))
malformed("wf-not-a-descriptor", A("x"), loc([], [], "not-descriptor"))
malformed("wf-field-is-not-a-descriptor", field("a", "req", ANY), loc([], [], "unknown-constructor"))
malformed("wf-union-garbage-alternative", d("union", d("atom"), A("garbage")), loc([], [2], "not-descriptor"))
malformed("wf-union-unknown-constructor", d("union", INT, d("frob")), loc([], [2], "unknown-constructor"))
malformed("wf-list-of-scalar-kind", d("list-of", d("scalar", A("float"))), loc([], [1], "argument", arg=1))
malformed("wf-tuple-first-child", d("tuple", d("scalar", A("float")), A("x")), loc([], [1], "argument", arg=1))
malformed(
    "wf-map-value-field-flag",
    d("map-of", ANY, d("record", A("open"), field("k", "maybe", ANY))),
    loc([], [2], "argument", arg=2),
)
malformed("wf-unbound-ref", d("ref", A("L")), loc([], [], "unbound-ref", arg=1))
malformed("wf-unbound-inner-ref", d("rec", A("L"), d("tuple", d("ref", A("M")))), loc([], [2, 1], "unbound-ref", arg=1))
malformed("wf-unguarded-ref", d("rec", A("L"), d("ref", A("L"))), loc([], [2], "unguarded-ref", arg=1))
malformed(
    "wf-own-form-beats-exhausted-child",
    d("record", A("closed"), field("a", "req", nest("not", 512, ANY)), A("x")),
    loc([], [], "argument", arg=3),
)
# Exhaustion beats a malformed sibling among children. The any below sits at depth 513.
EXHAUSTED_CHILD = {"verdict": "exhausted", "limit": "recursion_depth", "location": loc([], [1] * 513, "limit")}
case(
    "wf-exhausted-beats-malformed-sibling",
    d("tuple", nest("not", 512, ANY), A("x")),
    EXHAUSTED_CHILD,
    ("diagnose stops at the gate", A("x"), EXHAUSTED_CHILD),
)


def render():
    doc = {
        "version": "horos-diagnose-v1",
        "note": (
            "Hand-authored diagnostic locations (ADR 0009). `descriptor` and `value` are "
            "ontos-codec-v1 lowercase hex; `wf` is the expected diagnoseWf result and each "
            "`diagnosis` the expected diagnose result, in the JSON form of "
            "docs/spec/horos.md#diagnostic-locations, written from the specification. "
            "Generated inputs only: vectors/diagnose.py encodes them."
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
            raise SystemExit(f"stale diagnostic corpus: {', '.join(stale)}; run vectors/diagnose.py")
        print(f"diagnostic corpus current: {len(CASES)} cases")
        return
    for p in targets:
        p.write_text(text, encoding="utf-8")
    print(f"wrote {len(CASES)} cases")


if __name__ == "__main__":
    main()
