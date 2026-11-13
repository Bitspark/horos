"""Source of vectors/mixed-inhabitation.json, the hand-authored corpus for mixed inhabitation.

The corpus pins what it means for an ontos/wire mixed value to inhabit a
materialized Descriptor (ADR 0012, docs/spec/mixed-inhabitation.md). Horos ships
no checker for this judgment: a binding backend implements it and replays this
corpus. Every expected verdict below is written from the specification, never
from checker output. This script only encodes the inputs and checks their form.

Descriptors are ground ontos values, so they are ontos-codec-v1 hex as in the
other corpora. A mixed value has no codec: 0007 defines no portable encoding for
a capability. Values therefore use ontos's value-tree notation,
{"atom": "<hex>"} and {"tuple": [...]}, with one third leaf, {"wire": "<name>"},
for a capability. Within one value, equal names denote the same supplied handle
and different names distinct handles. No verdict depends on a name.

Run:  ONTOS_PY_ROOT=<ontos v0.17.0 checkout> python3 vectors/mixed-inhabitation.py [--check]
"""

from __future__ import annotations

import json
import os
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
OUT = ROOT / "vectors" / "mixed-inhabitation.json"
# The Go declarations tests check every descriptor's form from this copy, which
# ships inside the Go module so that its own tests can run after download.
COPIES = [ROOT / "core" / "go" / "testdata" / "mixed-inhabitation.json"]

_ONTOS = os.environ.get("ONTOS_PY_ROOT")
if not _ONTOS:
    raise SystemExit("Set ONTOS_PY_ROOT to an ontos v0.17.0 checkout")
for sub in ("core/py", "codec/py", "data/py"):
    sys.path.insert(0, str(Path(_ONTOS) / sub))

from ontos_codec import encode  # noqa: E402
from ontos_core import Atom, Tuple, atom, tuple_  # noqa: E402
from ontos_data import encode_int, encode_list, encode_map, encode_null, encode_set, encode_text  # noqa: E402

VERDICTS = ("conforms", "mismatch", "exhausted")


# Descriptors (ground values).
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


INT = d("scalar", A("int"))
TEXT = d("scalar", A("utf8-text"))
UNIT = d("tuple")


def wire(message):
    return d("wire", message)


def function(arg, result):
    """Function<A,B> = Wire<tuple(A, Wire<B>)>, as in docs/spec/wire-types.md."""
    return wire(d("tuple", arg, wire(result)))


# Mixed values: a ground ontos value, a capability leaf or a tuple of mixed values.
class Wire:
    def __init__(self, name):
        self.name = name


class Mixed:
    def __init__(self, items):
        self.items = list(items)


def W(name):
    return Wire(name)


def mt(*items):
    return Mixed(items)


def mlist(*items):
    return mt(A("list"), *items)


def mmap(*entries):
    """A mixed canonical map with text keys, sorted by the keys' codec encoding."""
    ordered = sorted(entries, key=lambda e: encode(T(e[0])))
    return mt(A("map"), *(mt(T(k), v) for k, v in ordered))


def mmap_unsorted(*entries):
    return mt(A("map"), *(mt(T(k), v) for k, v in entries))


def tree(v):
    if isinstance(v, Wire):
        return {"wire": v.name}
    if isinstance(v, Mixed):
        return {"tuple": [tree(x) for x in v.items]}
    if isinstance(v, Atom):
        return {"atom": v.bytes().hex()}
    if isinstance(v, Tuple):
        return {"tuple": [tree(x) for x in v.items()]}
    raise TypeError(f"not a mixed value: {v!r}")


def nested(depth, leaf):
    """A ground tuple nested `depth` times around `leaf`."""
    for _ in range(depth):
        leaf = tuple_([leaf])
    return leaf


CASES = []


def case(name, descriptor, note, *values):
    CASES.append(
        {
            "name": name,
            "note": note,
            "descriptor": encode(descriptor).hex(),
            "values": [{"name": n, "value": tree(v), "verdict": verdict} for n, v, verdict in values],
        }
    )


# --- A capability position admits exactly a capability leaf (decisions 3 and 4).
case(
    "wire-root",
    wire(INT),
    "wire(E) admits exactly a capability leaf; data, even reference-looking bytes, never inhabits it",
    ("a capability", W("w"), "conforms"),
    ("bytes that look like a reference", atom(b"wire:42"), "mismatch"),
    ("an int", I(42), "mismatch"),
    ("the empty tuple", tuple_([]), "mismatch"),
    ("a tuple holding a capability", mt(W("w")), "mismatch"),
)
case(
    "wire-message-not-consulted",
    wire(d("record", A("closed"), field("x", "req", INT))),
    "the message type E is not checked against the opaque leaf",
    ("a capability", W("w"), "conforms"),
    ("a value of the message type", mmap(("x", I(1))), "mismatch"),
)
case(
    "wire-empty-message",
    wire(d("union")),
    "even a message type with no values leaves the position satisfied by any capability",
    ("a capability", W("w"), "conforms"),
)

# --- Tuples, compounds and lists lift the ground rules.
case(
    "tuple-text-wire",
    d("tuple", TEXT, wire(UNIT)),
    "exact arity; each item inhabits its child",
    ("text then a capability", mt(T("a"), W("w")), "conforms"),
    ("data at the capability position", mt(T("a"), atom(b"")), "mismatch"),
    ("a capability at the text position", mt(W("v"), W("w")), "mismatch"),
    ("too short", mt(T("a")), "mismatch"),
    ("too long", mt(T("a"), W("w"), W("w")), "mismatch"),
    ("a bare capability", W("w"), "mismatch"),
)
case(
    "tuple-aliasing",
    d("tuple", wire(INT), wire(INT)),
    "inhabitation never compares capabilities: aliasing changes nothing",
    ("one handle twice", mt(W("w"), W("w")), "conforms"),
    ("two distinct handles", mt(W("w1"), W("w2")), "conforms"),
)
case(
    "compound-call",
    d("compound", A("call"), wire(INT), INT),
    "the exact label atom, then exactly the matching children",
    ("matching", mt(A("call"), W("w"), I(1)), "conforms"),
    ("another label", mt(A("cast"), W("w"), I(1)), "mismatch"),
    ("the label as text", mt(T("call"), W("w"), I(1)), "mismatch"),
    ("a missing child", mt(A("call"), W("w")), "mismatch"),
    ("children swapped", mt(A("call"), I(1), W("w")), "mismatch"),
)
case(
    "list-of-wire",
    d("list-of", wire(INT)),
    "a list-labelled tuple whose elements all inhabit the element type",
    ("empty", mlist(), "conforms"),
    ("three capabilities, one repeated", mlist(W("a"), W("b"), W("a")), "conforms"),
    ("a capability and an int", mlist(W("a"), I(1)), "mismatch"),
    ("a bare tuple of capabilities", mt(W("a"), W("b")), "mismatch"),
    ("a set label", mt(A("set"), W("a")), "mismatch"),
)

# --- Closed records lift the ground field rules over a canonical text-keyed map.
CELL_INT = d(
    "record",
    A("closed"),
    field("get", "req", function(UNIT, INT)),
    field("set", "req", function(INT, UNIT)),
)
case(
    "record-cell",
    CELL_INT,
    "Cell<int> from docs/spec/wire-types.md, materialized: get and set are capabilities",
    ("both fields", mmap(("get", W("g")), ("set", W("s"))), "conforms"),
    ("one handle for both fields", mmap(("get", W("w")), ("set", W("w"))), "conforms"),
    ("a required field missing", mmap(("get", W("g"))), "mismatch"),
    ("an extra field in a closed record", mmap(("get", W("g")), ("set", W("s")), ("peek", W("p"))), "mismatch"),
    ("data in a capability field", mmap(("get", W("g")), ("set", T("x"))), "mismatch"),
    ("entries out of canonical order", mmap_unsorted(("set", W("s")), ("get", W("g"))), "mismatch"),
    ("an atom key instead of text", mt(A("map"), mt(A("get"), W("g")), mt(T("set"), W("s"))), "mismatch"),
    ("not a map", mt(W("g"), W("s")), "mismatch"),
)
case(
    "record-optional-wire",
    d("record", A("closed"), field("name", "req", TEXT), field("notify", "opt", wire(TEXT))),
    "an optional capability field may be absent; present null is not absent",
    ("absent", mmap(("name", T("a"))), "conforms"),
    ("present", mmap(("name", T("a")), ("notify", W("n"))), "conforms"),
    ("present null", mmap(("name", T("a")), ("notify", encode_null())), "mismatch"),
)

# --- Unions: at least one alternative inhabits.
case(
    "union-wire-or-int",
    d("union", wire(INT), INT),
    "a capability or an int",
    ("a capability", W("w"), "conforms"),
    ("an int", I(5), "conforms"),
    ("text", T("5"), "mismatch"),
    ("a tuple holding a capability", mt(W("w")), "mismatch"),
)
case(
    "union-step",
    d("union", d("compound", A("item"), wire(INT)), d("compound", A("end"))),
    "Step<Wire<int>> from docs/spec/wire-types.md",
    ("an item carrying a capability", mt(A("item"), W("w")), "conforms"),
    ("the end", mt(A("end")), "conforms"),
    ("an item carrying data", mt(A("item"), I(3)), "mismatch"),
)

# --- Capability-free positions are ground conformance over the recovered value (decisions 2 and 5).
case(
    "any-is-data-only",
    d("tuple", d("any"), wire(INT)),
    "any keeps its ground meaning: every ground value, no capability",
    ("an int beside a capability", mt(I(1), W("w")), "conforms"),
    ("a list beside a capability", mt(encode_list([T("x"), T("y")]), W("w")), "conforms"),
    ("a capability at any", mt(W("v"), W("w")), "mismatch"),
    ("a capability nested under any", mt(mt(W("v")), W("w")), "mismatch"),
)
case(
    "open-record-is-data-only",
    d("tuple", d("record", A("open"), field("name", "req", TEXT)), wire(INT)),
    "an open record's extra fields are any, so they hold ground values only",
    ("a ground extra field", mt(mmap(("extra", I(1)), ("name", T("a"))), W("w")), "conforms"),
    ("a capability in an extra field", mt(mmap(("extra", W("v")), ("name", T("a"))), W("w")), "mismatch"),
)
NATS = d(
    "rec",
    A("L"),
    d("union", d("compound", A("nil")), d("compound", A("cons"), INT, d("ref", A("L")))),
)
case(
    "recursive-data-beside-wire",
    d("tuple", NATS, wire(INT)),
    "a recursive data subtree is checked by the ground judgment, ref scoping included",
    ("a two-element list", mt(tuple_([A("cons"), I(1), tuple_([A("cons"), I(2), tuple_([A("nil")])])]), W("w")), "conforms"),
    ("a capability inside the recursive data", mt(mt(A("cons"), W("v"), mt(A("nil"))), W("w")), "mismatch"),
)
case(
    "set-of-is-data-only",
    d("tuple", d("set-of", INT), wire(INT)),
    "set-of has a Data operand, so a capability element never inhabits it",
    ("a set of ints", mt(encode_set([I(1), I(2)]), W("w")), "conforms"),
    ("a capability in the set", mt(mt(A("set"), W("v")), W("w")), "mismatch"),
)
case(
    "wire-free-descriptor",
    d("record", A("closed"), field("n", "req", INT)),
    "a descriptor without wire is ground conformance of the recovered value",
    ("a ground record", mmap(("n", I(1))), "conforms"),
    ("a capability in the field", mmap(("n", W("w"))), "mismatch"),
    ("an extra field", mmap(("m", I(2)), ("n", I(1))), "mismatch"),
)
LITERAL_WIRE = d("wire", d("any"))
case(
    "literal-shaped-like-wire",
    d("tuple", d("exact", LITERAL_WIRE), wire(INT)),
    "an exact literal is an operand, not a type position: a literal shaped like wire(any) is data",
    ("the literal and a capability", mt(LITERAL_WIRE, W("w")), "conforms"),
    ("a capability where the literal is", mt(W("v"), W("w")), "mismatch"),
)

# --- Outcomes combine as in ADR 0008; exhaustion comes from a delegated ground check.
DEEP = nested(600, atom(b""))
CHAIN = d("rec", A("X"), d("union", d("atom"), d("tuple", d("ref", A("X")))))
case(
    "exhausted-ground-part",
    d("tuple", CHAIN, wire(INT)),
    "the ground check of a 600-deep value exceeds the depth bound; a decisive mismatch elsewhere still decides",
    ("deep data beside a capability", mt(DEEP, W("w")), "exhausted"),
    ("deep data beside data at the capability position", mt(DEEP, atom(b"")), "mismatch"),
    ("shallow data beside a capability", mt(nested(3, atom(b"")), W("w")), "conforms"),
)
case(
    "exhausted-union",
    d("union", CHAIN, wire(INT)),
    "a union conforms if any alternative conforms; otherwise exhaustion takes precedence over mismatch",
    ("deep data", DEEP, "exhausted"),
    ("a capability", W("w"), "conforms"),
    ("text", T("x"), "mismatch"),
)


# --- Form checks: the corpus is well formed before it is written or compared.
_HEX = re.compile(r"^(?:[0-9a-f]{2})*$")
_NAME = re.compile(r"^[a-z0-9][a-z0-9-]*$")


def check_tree(node, path):
    if not isinstance(node, dict) or len(node) != 1:
        raise SystemExit(f"{path}: a value is exactly one of atom, tuple or wire")
    (kind, body), = node.items()
    if kind == "atom":
        if not isinstance(body, str) or not _HEX.match(body):
            raise SystemExit(f"{path}: atom bytes must be lowercase even-length hex")
    elif kind == "tuple":
        if not isinstance(body, list):
            raise SystemExit(f"{path}: tuple items must be a list")
        for i, item in enumerate(body):
            check_tree(item, f"{path}/{i}")
    elif kind == "wire":
        if not isinstance(body, str) or not body:
            raise SystemExit(f"{path}: a wire leaf has a nonempty name")
    else:
        raise SystemExit(f"{path}: unknown leaf {kind!r}")


def check_form():
    names = set()
    for c in CASES:
        if not _NAME.match(c["name"]) or c["name"] in names:
            raise SystemExit(f"case name {c['name']!r} is malformed or repeated")
        names.add(c["name"])
        if not c["values"]:
            raise SystemExit(f"{c['name']}: no values")
        seen = set()
        for v in c["values"]:
            if v["verdict"] not in VERDICTS or v["name"] in seen:
                raise SystemExit(f"{c['name']} / {v['name']}: bad verdict or repeated name")
            seen.add(v["name"])
            check_tree(v["value"], f"{c['name']} / {v['name']}")


def render():
    check_form()
    doc = {
        "format": "horos-mixed-inhabitation-v1",
        "specification": "docs/spec/mixed-inhabitation.md",
        "decision": "docs/architecture/decisions/0012-mixed-inhabitation.md",
        "value_notation": (
            'ontos value tree {"atom": hex} | {"tuple": [...]}, plus {"wire": name} for a capability leaf; '
            "equal names within one value denote the same handle, and no verdict depends on a name"
        ),
        "cases": CASES,
    }
    # One line per value keeps deep values compact and diffs readable.
    lines = ["{"]
    for key in ("format", "specification", "decision", "value_notation"):
        lines.append(f"  {json.dumps(key)}: {json.dumps(doc[key])},")
    lines.append('  "cases": [')
    for ci, c in enumerate(CASES):
        lines.append("    {")
        for key in ("name", "note", "descriptor"):
            lines.append(f"      {json.dumps(key)}: {json.dumps(c[key])},")
        lines.append('      "values": [')
        for vi, v in enumerate(c["values"]):
            sep = "," if vi < len(c["values"]) - 1 else ""
            lines.append("        " + json.dumps(v, separators=(", ", ": ")) + sep)
        lines.append("      ]")
        lines.append("    }" + ("," if ci < len(CASES) - 1 else ""))
    lines.append("  ]")
    lines.append("}")
    text = "\n".join(lines) + "\n"
    if json.loads(text)["cases"] != CASES:
        raise SystemExit("rendered corpus does not round-trip")
    return text


def main():
    text = render()
    targets = [OUT, *COPIES]
    if "--check" in sys.argv[1:]:
        stale = [str(p.relative_to(ROOT)) for p in targets if not p.is_file() or p.read_text(encoding="utf-8") != text]
        if stale:
            raise SystemExit(f"stale mixed-inhabitation corpus: {', '.join(stale)}; run vectors/mixed-inhabitation.py")
        print(f"mixed-inhabitation corpus current: {len(CASES)} cases")
        return
    for p in targets:
        p.write_text(text, encoding="utf-8")
    print(f"wrote {len(CASES)} cases")


if __name__ == "__main__":
    main()
