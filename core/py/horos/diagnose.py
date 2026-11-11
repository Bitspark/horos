"""Diagnostic outcomes and locations for the Python validation lane.

``diagnose_wf(T)`` and ``diagnose(v, T)`` report one of four verdicts, ADR 0008:
``conforms``, ``mismatch``, ``not_well_formed`` or ``exhausted``. An exhausted
check names its limit, and ``checking_profile()`` gives the limits a verdict is
stated under. Every verdict other than ``conforms`` carries one location
(ADR 0009, docs/spec/horos.md "Diagnostic locations"). The boolean ``wf`` and
``conforms`` in this package are unchanged.

This module is written from the contract and the decision records. It is the
fourth independent reading of them, not a port of another core. Leaf acceptance
is the boolean recognizer's own leaf rule, because the contract defines a leaf
diagnosis as its acceptance. Composition, verdict precedence and locations are
implemented here.

The reading of the contract, rule by rule:

- Every node of a descriptor, the root included, is entered at a depth. A child
  descriptor, a ``rec`` body and the body a ``ref`` unfolds are entered one level
  deeper. Entering a node deeper than ``MAX_RECURSION_DEPTH`` is exhaustion, and
  its location is that node.
- Structural children (tuple items, collection elements, map keys and values,
  record fields) form a conjunction, as ``and`` does. A mismatching child decides
  a mismatch even when an earlier sibling is exhausted. Otherwise the first
  exhausted child decides exhaustion.
- An exhausted union takes the location of its first exhausted alternative, in
  item order. The longest-path rule chooses between mismatching alternatives only.
- A ``ref`` resolves to the innermost binding of its name in the current unfolding,
  as the boolean recognizer does. Completed diagnoses must agree with the
  boolean. A ``ref`` reached again before the value descends, which that
  resolution allows in a well-formed descriptor, is refused at the ``ref``.

Like the boolean functions, these never raise for any pair of ontos values, but
the host interpreter's recursion limit can be reached before the logical depth
bound on deep input (docs/spec/horos.md, "Bounded checking and diagnostics").
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import List, Optional, Tuple

from ontos_core import Value
from ontos_data import DataError, read_int, read_list, read_map, read_set, read_text

from . import (
    ATOM_LEN_BOUND_MAX,
    ATOM_LEN_BOUND_MIN,
    MAX_RECURSION_DEPTH,
    _conforms_in,
    _head,
    _read_field,
)

__all__ = [
    "CONFORMS",
    "MISMATCH",
    "NOT_WELL_FORMED",
    "EXHAUSTED",
    "RECURSION_DEPTH",
    "Step",
    "Location",
    "Diagnosis",
    "Profile",
    "checking_profile",
    "diagnose_wf",
    "diagnose",
]

#: The four verdicts (ADR 0008).
CONFORMS = "conforms"
MISMATCH = "mismatch"
NOT_WELL_FORMED = "not_well_formed"
EXHAUSTED = "exhausted"

#: The limit an ``exhausted`` diagnosis reached: the recursive-descent bound.
RECURSION_DEPTH = "recursion_depth"


@dataclass(frozen=True)
class Step:
    """One step of a value path.

    ``kind`` is ``item`` (bare tuple), ``elem`` (list or set element), ``key`` or
    ``value`` (the two sides of a map entry), each with a 0-based ``index`` in
    canonical order, or ``field`` with the record field's ``name``.
    """

    kind: str
    index: Optional[int] = None
    name: Optional[str] = None

    def to_json(self) -> dict:
        if self.kind == "field":
            return {"kind": "field", "name": self.name}
        return {"kind": self.kind, "index": self.index}


@dataclass(frozen=True)
class Location:
    """Where a check failed.

    ``path`` leads from the checked value to the value at fault, ``at`` is the
    tuple-item position of the descriptor at fault in the written descriptor,
    and ``reason`` says why. ``field`` names the record field of
    ``missing-field`` and ``extra-field``. ``arg`` is the item index, within the
    descriptor at ``at``, of the faulty argument of ``argument``,
    ``duplicate-field``, ``unbound-ref`` and ``unguarded-ref``.
    """

    path: Tuple[Step, ...]
    at: Tuple[int, ...]
    reason: str
    field: Optional[str] = None
    arg: Optional[int] = None

    def to_json(self) -> dict:
        out = {"path": [s.to_json() for s in self.path], "at": list(self.at), "reason": self.reason}
        if self.field is not None:
            out["field"] = self.field
        if self.arg is not None:
            out["arg"] = self.arg
        return out


@dataclass(frozen=True)
class Diagnosis:
    """A checking outcome. ``location`` is set for every verdict but ``conforms``;
    ``limit`` is set for ``exhausted`` only."""

    verdict: str
    location: Optional[Location] = None
    limit: Optional[str] = None

    @property
    def complete(self) -> bool:
        """Whether this is a completed decision. ``exhausted`` is not."""
        return self.verdict != EXHAUSTED

    def completed_bool(self) -> Optional[bool]:
        """The boolean a completed outcome corresponds to, or None when exhausted:
        an exhausted check establishes neither membership nor non-membership."""
        if self.verdict == CONFORMS:
            return True
        if self.verdict in (MISMATCH, NOT_WELL_FORMED):
            return False
        return None

    def to_json(self) -> dict:
        """The JSON form of the form contract, as vectors/diagnose.json pins it."""
        out: dict = {"verdict": self.verdict}
        if self.limit is not None:
            out["limit"] = self.limit
        if self.location is not None:
            out["location"] = self.location.to_json()
        return out


@dataclass(frozen=True)
class Profile:
    """The limits a diagnosis is stated under. A completed negative is completed
    under this profile. ``bound_min`` and ``bound_max`` delimit every integer
    bound a descriptor may state: ``atom-len``, ``text-len``, ``int-min`` and
    ``int-max``."""

    max_recursion_depth: int
    bound_min: int
    bound_max: int


def checking_profile() -> Profile:
    """The profile this implementation decides under."""
    return Profile(MAX_RECURSION_DEPTH, ATOM_LEN_BOUND_MIN, ATOM_LEN_BOUND_MAX)


# An internal outcome is None for conforms (or well formed), else (verdict, Location).
_Outcome = Optional[Tuple[str, Location]]


def _finish(outcome: _Outcome) -> Diagnosis:
    if outcome is None:
        return Diagnosis(CONFORMS)
    verdict, location = outcome
    return Diagnosis(verdict, location, RECURSION_DEPTH if verdict == EXHAUSTED else None)


# ---------------------------------------------------------------------------
# well-formedness
# ---------------------------------------------------------------------------

# Arity as (minimum, maximum or None for unbounded) argument count. `field` is a
# record member form, not a constructor, so it is absent.
_ARITY = {
    "any": (0, 0),
    "atom": (0, 0),
    "key-grammar": (0, 0),
    "text-non-blank": (0, 0),
    "lowercase-uuid": (0, 0),
    "atom-exact": (1, 1),
    "atom-in": (1, 1),
    "scalar": (1, 1),
    "int-min": (1, 1),
    "int-max": (1, 1),
    "seq": (1, 1),
    "list-of": (1, 1),
    "set-of": (1, 1),
    "not": (1, 1),
    "exact": (1, 1),
    "ref": (1, 1),
    "atom-len": (2, 2),
    "text-len": (2, 2),
    "map-of": (2, 2),
    "rec": (2, 2),
    "tuple": (0, None),
    "union": (0, None),
    "and": (0, None),
    "compound": (1, None),
    "record": (1, None),
}

_SCALAR_KINDS = (b"int", b"utf8-text", b"bool")
_RECORD_MODES = (b"open", b"closed")


def _bound(v: Value) -> Optional[int]:
    """A descriptor's integer bound: a canonical int in the signed 128-bit range."""
    try:
        n = read_int(v)
    except DataError:
        return None
    return n if ATOM_LEN_BOUND_MIN <= n <= ATOM_LEN_BOUND_MAX else None


def _own_fault(ctor: str, args: List[Value], bound: Tuple[bytes, ...], pending: Tuple[bytes, ...]):
    """The first fault in a descriptor's arguments, each checked in item order against
    those before it, as (reason, item index); None when its own form is sound."""
    if ctor in ("atom-exact", "compound", "rec"):
        if not args[0].is_atom:
            return "argument", 1
    elif ctor == "atom-in":
        try:
            members = read_set(args[0])
        except DataError:
            return "argument", 1
        if not all(m.is_atom for m in members):
            return "argument", 1
    elif ctor == "scalar":
        if not (args[0].is_atom and args[0].bytes() in _SCALAR_KINDS):
            return "argument", 1
    elif ctor in ("int-min", "int-max"):
        if _bound(args[0]) is None:
            return "argument", 1
    elif ctor in ("atom-len", "text-len"):
        lo = _bound(args[0])
        if lo is None or lo < 0:
            return "argument", 1
        hi = _bound(args[1])
        if hi is None or hi < lo:
            return "argument", 2
    elif ctor == "record":
        if not (args[0].is_atom and args[0].bytes() in _RECORD_MODES):
            return "argument", 1
        names = set()
        for item, form in enumerate(args[1:], start=2):
            fld = _read_field(form)
            if fld is None:
                return "argument", item
            if fld[0] in names:
                return "duplicate-field", item
            names.add(fld[0])
    elif ctor == "ref":
        if not args[0].is_atom:
            return "argument", 1
        name = args[0].bytes()
        if name not in bound:
            return "unbound-ref", 1
        if name in pending:
            return "unguarded-ref", 1
    return None


def _wf_children(ctor: str, args: List[Value], at, bound, pending):
    """The child descriptors in item order, as (descriptor, position, bound, pending).
    A value descent clears the guard set; union, and and not keep it; rec extends both."""
    if ctor in ("tuple", "seq", "list-of", "set-of", "map-of"):
        return [(c, at + (i,), bound, ()) for i, c in enumerate(args, start=1)]
    if ctor == "compound":
        return [(c, at + (i,), bound, ()) for i, c in enumerate(args[1:], start=2)]
    if ctor == "record":
        return [(_read_field(f)[2], at + (i, 3), bound, ()) for i, f in enumerate(args[1:], start=2)]
    if ctor in ("union", "and", "not"):
        return [(c, at + (i,), bound, pending) for i, c in enumerate(args, start=1)]
    if ctor == "rec":
        name = args[0].bytes()
        return [(args[1], at + (2,), bound + (name,), pending + (name,))]
    return []


def _wf(t: Value, at: Tuple[int, ...], bound, pending, depth: int) -> _Outcome:
    if depth > MAX_RECURSION_DEPTH:
        return EXHAUSTED, Location((), at, "limit")
    head = _head(t)
    if head is None:
        return NOT_WELL_FORMED, Location((), at, "not-descriptor")
    ctor, args = head
    arity = _ARITY.get(ctor)
    if arity is None:
        return NOT_WELL_FORMED, Location((), at, "unknown-constructor")
    least, most = arity
    if len(args) < least or (most is not None and len(args) > most):
        return NOT_WELL_FORMED, Location((), at, "arity")
    fault = _own_fault(ctor, args, bound, pending)
    if fault is not None:
        return NOT_WELL_FORMED, Location((), at, fault[0], arg=fault[1])
    # Own form is sound. Among the children, the first exhausted one decides; else the
    # first malformed one.
    malformed: _Outcome = None
    for child, child_at, child_bound, child_pending in _wf_children(ctor, args, at, bound, pending):
        outcome = _wf(child, child_at, child_bound, child_pending, depth + 1)
        if outcome is None:
            continue
        if outcome[0] == EXHAUSTED:
            return outcome
        if malformed is None:
            malformed = outcome
    return malformed


def diagnose_wf(t: Value) -> Diagnosis:
    """Diagnose ``wf(T)``: conforms (well formed), not_well_formed or exhausted."""
    return _finish(_wf(t, (), (), (), 0))


# ---------------------------------------------------------------------------
# conformance
# ---------------------------------------------------------------------------

# Leaf descriptors accept or refuse the value itself, with no child descriptor.
_LEAVES = frozenset(
    (
        "any",
        "atom",
        "atom-exact",
        "atom-len",
        "atom-in",
        "scalar",
        "int-min",
        "int-max",
        "text-len",
        "key-grammar",
        "text-non-blank",
        "lowercase-uuid",
        "exact",
    )
)


def _refused(path, at) -> _Outcome:
    return MISMATCH, Location(path, at, "refused")


def _lookup(env, name: bytes):
    """The innermost binding of ``name``: (body, body position), or None."""
    while env is not None:
        (bound_name, body, body_at), env = env
        if bound_name == name:
            return body, body_at
    return None


def _record(v: Value, args: List[Value], path, at):
    """A record's own checks, in order: canonical map, text keys, missing required
    fields, extra fields of a closed record. Returns (failure, None) or (None, children)."""
    try:
        entries = read_map(v)
    except DataError:
        return _refused(path, at), None
    present = {}
    keys = []
    for index, (k, value) in enumerate(entries):
        try:
            name = read_text(k)
        except DataError:
            return _refused(path + (Step("key", index),), at), None
        present[name] = value
        keys.append(name)
    fields = [(item,) + _read_field(f) for item, f in enumerate(args[1:], start=2)]
    for _item, name, required, _ty in fields:
        if required and name not in present:
            return (MISMATCH, Location(path, at, "missing-field", field=name)), None
    if args[0].bytes() == b"closed":
        declared = {name for _item, name, _required, _ty in fields}
        for name in keys:
            if name not in declared:
                return (MISMATCH, Location(path, at, "extra-field", field=name)), None
    children = [
        (present[name], ty, path + (Step("field", name=name),), at + (item, 3))
        for item, name, _required, ty in fields
        if name in present
    ]
    return None, children


def _check(v: Value, t: Value, path, at, env, pending: Tuple[bytes, ...], depth: int) -> _Outcome:
    if depth > MAX_RECURSION_DEPTH:
        return EXHAUSTED, Location(path, at, "limit")
    ctor, args = _head(t)  # the gate admitted t, so every node reached is a descriptor

    if ctor in _LEAVES:
        return None if _conforms_in(v, t, None, None, 0) else _refused(path, at)

    if ctor == "union":
        exhausted: _Outcome = None
        deepest: _Outcome = None
        for i, alternative in enumerate(args, start=1):
            outcome = _check(v, alternative, path, at + (i,), env, pending, depth + 1)
            if outcome is None:
                return None
            if outcome[0] == EXHAUSTED:
                if exhausted is None:
                    exhausted = outcome
            elif deepest is None or len(outcome[1].path) > len(deepest[1].path):
                deepest = outcome
        if exhausted is not None:
            return exhausted
        if deepest is not None and len(deepest[1].path) > len(path):
            return deepest
        return MISMATCH, Location(path, at, "no-alternative")

    if ctor == "not":
        outcome = _check(v, args[0], path, at + (1,), env, pending, depth + 1)
        if outcome is None:
            return MISMATCH, Location(path, at, "complement")
        return None if outcome[0] == MISMATCH else outcome

    if ctor == "rec":
        name = args[0].bytes()
        body_at = at + (2,)
        return _check(v, args[1], path, body_at, ((name, args[1], body_at), env), pending + (name,), depth + 1)

    if ctor == "ref":
        name = args[0].bytes()
        binding = None if name in pending else _lookup(env, name)
        if binding is None:
            return _refused(path, at)
        body, body_at = binding
        return _check(v, body, path, body_at, env, pending + (name,), depth + 1)

    # The rest are conjunctions: and over the same value, the others over parts of
    # it after their own checks. Each child is (value, descriptor, path, position).
    children: list
    child_pending: Tuple[bytes, ...] = ()
    if ctor == "and":
        children = [(v, c, path, at + (i,)) for i, c in enumerate(args, start=1)]
        child_pending = pending
    elif ctor == "tuple":
        if not v.is_tuple or len(v.items()) != len(args):
            return _refused(path, at)
        children = [(x, c, path + (Step("item", i),), at + (i + 1,)) for i, (x, c) in enumerate(zip(v.items(), args))]
    elif ctor == "seq":
        if not v.is_tuple:
            return _refused(path, at)
        children = [(x, args[0], path + (Step("item", i),), at + (1,)) for i, x in enumerate(v.items())]
    elif ctor == "compound":
        items = v.items() if v.is_tuple else None
        if items is None or len(items) != len(args) or not items[0].is_atom or items[0].bytes() != args[0].bytes():
            return _refused(path, at)
        children = [(items[i], args[i], path + (Step("item", i),), at + (i + 1,)) for i in range(1, len(args))]
    elif ctor in ("list-of", "set-of"):
        try:
            elements = read_list(v) if ctor == "list-of" else read_set(v)
        except DataError:
            return _refused(path, at)
        children = [(x, args[0], path + (Step("elem", i),), at + (1,)) for i, x in enumerate(elements)]
    elif ctor == "map-of":
        try:
            entries = read_map(v)
        except DataError:
            return _refused(path, at)
        children = []
        for i, (k, value) in enumerate(entries):
            children.append((k, args[0], path + (Step("key", i),), at + (1,)))
            children.append((value, args[1], path + (Step("value", i),), at + (2,)))
    elif ctor == "record":
        failure, children = _record(v, args, path, at)
        if failure is not None:
            return failure
    else:  # pragma: no cover - the gate admits no other constructor
        raise AssertionError(f"diagnose reached constructor {ctor!r} past the well-formedness gate")

    # A mismatching child decides; otherwise the first exhausted child does.
    exhausted = None
    for child_v, child_t, child_path, child_at in children:
        outcome = _check(child_v, child_t, child_path, child_at, env, child_pending, depth + 1)
        if outcome is None:
            continue
        if outcome[0] == MISMATCH:
            return outcome
        if exhausted is None:
            exhausted = outcome
    return exhausted


def diagnose(v: Value, t: Value) -> Diagnosis:
    """Diagnose ``v : T``. A descriptor that is not well formed, or whose check is
    exhausted, stops at the gate with that diagnosis and an empty value path."""
    gate = _wf(t, (), (), (), 0)
    if gate is not None:
        return _finish(gate)
    return _finish(_check(v, t, (), (), None, (), 0))
