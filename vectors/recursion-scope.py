"""Source of vectors/recursion-scope.json, the hand-authored oracle for ref scoping (ADR 0011).

A `ref` denotes the `rec` that lexically encloses it in the written descriptor: its
nearest enclosing binding of that name. Unfolding a `ref` continues in its binding's own
environment, so the free refs of the unfolded body keep the bindings they were written
under, whatever `rec`s enclose the place where the unfolding happens. Every expected
`wf`, `conforms`, `diagnose_wf` and `diagnosis` below is written from that rule,
docs/spec/horos.md and ADRs 0008, 0009 and 0011, never from checker output. This script
only encodes the inputs as ontos-codec-v1 hex.

The original oracle, vectors/conformance.json, is pinned byte for byte, so these
judgments live here, in its format, with the diagnostic location of each judgment beside
the boolean: `wf` and `conforms` are booleans, `diagnose_wf` and `diagnosis` are the
JSON form of docs/spec/horos.md#diagnostic-locations.

Run:  ONTOS_PY_ROOT=<ontos v0.17.0 checkout> python3 vectors/recursion-scope.py [--check]
"""

from __future__ import annotations

import json
import os
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
OUT = ROOT / "vectors" / "recursion-scope.json"
COPIES = [ROOT / "core" / "go" / "testdata" / "recursion-scope.json", ROOT / "core" / "rs" / "testdata" / "recursion-scope.json"]

_ONTOS = os.environ.get("ONTOS_PY_ROOT")
if not _ONTOS:
    raise SystemExit("Set ONTOS_PY_ROOT to an ontos v0.17.0 checkout")
for sub in ("core/py", "codec/py", "data/py"):
    sys.path.insert(0, str(Path(_ONTOS) / sub))

from ontos_codec import encode  # noqa: E402
from ontos_core import atom, tuple_  # noqa: E402
from ontos_data import encode_int, encode_list  # noqa: E402

# ----- authored inputs ---------------------------------------------------------


def A(s):
    return atom(s.encode("utf-8"))


def I(n):  # noqa: E743
    return encode_int(n)


def L(*elements):
    return encode_list(list(elements))


def Tup(*items):
    return tuple_(list(items))


def d(ctor, *args):
    return tuple_([A("horos/" + ctor), *args])


def rec(name, body):
    return d("rec", A(name), body)


def ref(name):
    return d("ref", A(name))


def exact(name):
    return d("exact", A(name))


INT = d("scalar", A("int"))

# ----- authored expectations ---------------------------------------------------


def item(i):
    return {"kind": "item", "index": i}


def elem(i):
    return {"kind": "elem", "index": i}


def loc(path, at, reason, arg=None):
    out = {"path": path, "at": at, "reason": reason}
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


def case(name, descriptor, diagnose_wf, *values):
    """`values` are (note, value, conforms, diagnosis). The booleans are authored beside the
    diagnoses and must tell the same story: a completed diagnosis is the boolean, an
    exhausted check is the boolean's bounded false, and a malformed descriptor admits no
    value and stops `diagnose` at its gate."""
    wf = diagnose_wf == OK
    for note, _, conforms, diagnosis in values:
        if not wf:
            assert conforms is False and diagnosis == diagnose_wf, (name, note)
        elif diagnosis["verdict"] == "exhausted":
            assert conforms is False, (name, note)
        else:
            assert conforms == (diagnosis == OK), (name, note)
    CASES.append(
        {
            "name": name,
            "descriptor": encode(descriptor).hex(),
            "wf": wf,
            "diagnose_wf": diagnose_wf,
            "values": [
                {"note": note, "value": encode(v).hex(), "conforms": conforms, "diagnosis": diagnosis}
                for note, v, conforms, diagnosis in values
            ],
        }
    )


# The reported reproducer. Lexically: outer M = a | L, L = (outer M, inner M),
# inner M = b | (L). Positions: the outer M's body (a union) is [2], L's body (a tuple) is
# [2, 2, 2], the inner M's body (a union) is [2, 2, 2, 2, 2]. L's ref(M) continues at [2]
# wherever L is unfolded; checkers that unfold L inside the inner M read it as [2, 2, 2, 2, 2].
REPRODUCER = rec("M", d("union", exact("a"), rec("L", d("tuple", ref("M"), rec("M", d("union", exact("b"), d("tuple", ref("L"))))))))
assert encode(REPRODUCER).hex() == (
    "01030009686f726f732f72656300014d0103000b686f726f732f756e696f6e0102000b686f726f732f6578616374000161"
    "01030009686f726f732f72656300014c0103000b686f726f732f7475706c6501020009686f726f732f72656600014d0103"
    "0009686f726f732f72656300014d0103000b686f726f732f756e696f6e0102000b686f726f732f657861637400016201"
    "02000b686f726f732f7475706c6501020009686f726f732f72656600014c"
), "the reproducer as reported"
a, b = A("a"), A("b")
REPORTED = Tup(a, Tup(Tup(a, b)))
assert encode(REPORTED).hex() == "010200016101010102000161000162", "the value as reported"

REPRODUCER_VALUES = (
    ("a: the outer base case", a, True, OK),
    ("(a, b): the inner M checks b where it is written; nothing is unfolded inside it", Tup(a, b), True, OK),
    ("(a, ((a, b),)), as reported: L unfolded inside the inner M still reads ref(M) as the outer M, so a conforms", REPORTED, True, OK),
    (
        "(a, (((a, ((a, b),)), b),)): the outer M again, one unfolding deeper",
        Tup(a, Tup(Tup(REPORTED, b))),
        True,
        OK,
    ),
    (
        "(a, ((b, b),)), the twin: b is not an outer M, though it is an inner one; the outer union names itself",
        Tup(a, Tup(Tup(b, b))),
        False,
        MIS([item(1), item(0), item(0)], [2], "no-alternative"),
    ),
    (
        "(a, c): no alternative of the inner M where it is written",
        Tup(a, A("c")),
        False,
        MIS([item(1)], [2, 2, 2, 2, 2], "no-alternative"),
    ),
)
case("reproducer", REPRODUCER, OK, *REPRODUCER_VALUES)

# The same descriptor with its inner M renamed K. Nothing shadows, so every reading agrees,
# and lexical scoping makes the two descriptors answer alike: renaming a binding consistently
# changes no verdict and no location.
case(
    "reproducer-renamed",
    rec("M", d("union", exact("a"), rec("L", d("tuple", ref("M"), rec("K", d("union", exact("b"), d("tuple", ref("L")))))))),
    OK,
    *REPRODUCER_VALUES,
)

# The guard against unguarded unfolding identifies bindings, not names. Lexically:
# outer M = (m, L), L = outer M | (l, inner M), inner M = b | L. Checking a value against the
# inner M enters the inner M, unfolds L at the same value, and reaches L's ref(M): the outer
# M, which has not been entered at this value. A name-keyed guard refuses it, because a
# binding named M has been. Positions: the outer M's body (a compound) is [2], L's body
# (a union) is [2, 2, 2], the inner M's body (a union) is [2, 2, 2, 2, 2, 2].
GUARD = rec(
    "M",
    d("compound", A("m"), rec("L", d("union", ref("M"), d("compound", A("l"), rec("M", d("union", exact("b"), ref("L"))))))),
)
m, lab = A("m"), A("l")
case(
    "guard-identifies-bindings",
    GUARD,
    OK,
    ("(m, (l, b)): the inner M is b", Tup(m, Tup(lab, b)), True, OK),
    (
        "(m, (l, (m, (l, b)))): the inner M is an L, which is the outer M at the same value",
        Tup(m, Tup(lab, Tup(m, Tup(lab, b)))),
        True,
        OK,
    ),
    (
        "(m, (l, (m, (l, c)))): c is not the inner M's b, nor an L, four levels down",
        Tup(m, Tup(lab, Tup(m, Tup(lab, A("c"))))),
        False,
        MIS([item(1)] * 4, [2, 2, 2, 2, 2, 2], "no-alternative"),
    ),
)

# Shadowing three deep. Lexically: M1 = a | L, L = (M1, M2), M2 = b | K, K = (M2, M3),
# M3 = c | (L, K). L and K are both unfolded inside M3; L's ref(M) is M1 and K's is M2.
# Body positions: M1 [2], L [2, 2, 2], M2 [2, 2, 2, 2, 2], K [2]*7, M3 [2]*9.
THREE = rec(
    "M",
    d(
        "union",
        exact("a"),
        rec(
            "L",
            d(
                "tuple",
                ref("M"),
                rec(
                    "M",
                    d(
                        "union",
                        exact("b"),
                        rec("K", d("tuple", ref("M"), rec("M", d("union", exact("c"), d("tuple", ref("L"), ref("K")))))),
                    ),
                ),
            ),
        ),
    ),
)
c = A("c")
case(
    "shadowing-three-deep",
    THREE,
    OK,
    ("a", a, True, OK),
    ("(a, (b, c)): each binding once; nothing is unfolded under a shadow", Tup(a, Tup(b, c)), True, OK),
    (
        "(a, (b, ((a, b), (b, c)))): inside M3, L's ref(M) is M1 and K's is M2",
        Tup(a, Tup(b, Tup(Tup(a, b), Tup(b, c)))),
        True,
        OK,
    ),
    (
        "(a, (b, ((c, b), (c, c)))): c is an M3 but not the M1 that L's ref(M) denotes",
        Tup(a, Tup(b, Tup(Tup(c, b), Tup(c, c)))),
        False,
        MIS([item(1), item(1), item(0), item(0)], [2], "no-alternative"),
    ),
    (
        "(a, (b, ((a, b), (c, c)))): L holds; c is not the M2 that K's ref(M) denotes",
        Tup(a, Tup(b, Tup(Tup(a, b), Tup(c, c)))),
        False,
        MIS([item(1), item(1), item(1), item(0)], [2, 2, 2, 2, 2], "no-alternative"),
    ),
)

# Shadowing without capture: the inner binding's body has no free ref, so no reading can
# differ. This is the original oracle's rec-nested-shadow descriptor, byte for byte, with
# locations. Body positions: outer x [2], inner x [2, 2, 1, 2].
NESTED_SHADOW = rec(
    "x",
    d(
        "union",
        d("atom-exact", A("onil")),
        d("tuple", rec("x", d("union", d("atom-exact", A("inil")), d("tuple", INT, ref("x")))), ref("x")),
    ),
)
_PINNED = json.loads((ROOT / "vectors" / "conformance.json").read_text(encoding="utf-8"))
assert encode(NESTED_SHADOW).hex() == next(c["descriptor"] for c in _PINNED["cases"] if c["name"] == "rec-nested-shadow")
onil, inil = A("onil"), A("inil")
case(
    "shadowing-without-capture",
    NESTED_SHADOW,
    OK,
    ("onil", onil, True, OK),
    ("(inil, onil)", Tup(inil, onil), True, OK),
    ("((1, inil), onil)", Tup(Tup(I(1), inil), onil), True, OK),
    ("((1, (2, inil)), (inil, onil)): each ref keeps its own binding", Tup(Tup(I(1), Tup(I(2), inil)), Tup(inil, onil)), True, OK),
    ("(onil, onil): the inner x is not the outer one", Tup(onil, onil), False, MIS([item(0)], [2, 2, 1, 2], "no-alternative")),
    (
        "(inil, (onil, onil)): the outer ref, written outside the inner x, is the outer x",
        Tup(inil, Tup(onil, onil)),
        False,
        MIS([item(1), item(0)], [2, 2, 1, 2], "no-alternative"),
    ),
)

# The declarations layer's closed nested recursion: an application substitutes a closed
# recursive argument under a binding of the same name. Body positions: outer x [2],
# inner x [2, 1, 2].
case(
    "closed-argument-under-same-name",
    rec("x", d("list-of", rec("x", d("list-of", ref("x"))))),
    OK,
    ("[[[]]]", L(L(L())), True, OK),
    ("[[1]]: the inner x is a list", L(L(I(1))), False, MIS([elem(0), elem(0)], [2, 1, 2], "refused")),
)


# Exhaustion through shadowed recursion: the reproducer on a chain that unfolds L inside the
# inner M at every level. Level j's L tuple, checking (a, y) at path [item(1), item(0)] * j,
# is entered at depth 3 + 5j; its ref(M) reaches the outer M's exact(a) at 6 + 5j. With 101
# levels everything finishes by depth 511; with 102 the 102nd L tuple is entered at 513.
def chain(n):
    y = b
    for _ in range(n):
        y = Tup(Tup(a, y))
    return Tup(a, y)


case(
    "exhaustion-through-shadowing",
    REPRODUCER,
    OK,
    ("101 unfoldings of L inside the inner M complete", chain(101), True, OK),
    (
        "102 unfoldings reach the bound at the L tuple; the boolean is false",
        chain(102),
        False,
        EXH([item(1), item(0)] * 102, [2, 2, 2], "limit"),
    ),
)

# Well-formedness was already lexical and is unchanged: the inner ref(M) denotes the inner M,
# which it reaches without a descent, although the outer M is guarded.
case(
    "wf-shadowing-ref-unguarded",
    rec("M", d("tuple", rec("M", d("union", exact("b"), ref("M"))))),
    NWF([], [2, 1, 2, 2], "unguarded-ref", arg=1),
    ("any value", A("x"), False, NWF([], [2, 1, 2, 2], "unguarded-ref", arg=1)),
)


def render():
    doc = {
        "version": "horos-recursion-scope-v1",
        "note": (
            "Hand-authored oracle for ref scoping (ADR 0011): a ref denotes the rec that lexically "
            "encloses it. `descriptor` and `value` are ontos-codec-v1 lowercase hex; `wf`/`conforms` "
            "are the expected booleans and `diagnose_wf`/`diagnosis` the expected diagnoses in the "
            "JSON form of docs/spec/horos.md#diagnostic-locations, authored from the specification. "
            "Generated inputs only: vectors/recursion-scope.py encodes them."
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
            raise SystemExit(f"stale recursion-scope oracle: {', '.join(stale)}; run vectors/recursion-scope.py")
        print(f"recursion-scope oracle current: {len(CASES)} cases")
        return
    for p in targets:
        p.write_text(text, encoding="utf-8")
    print(f"wrote {len(CASES)} cases")


if __name__ == "__main__":
    main()
