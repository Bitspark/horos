"""Python validation lane for horos descriptors over ontos values.

Source-only; replays the same conformance corpus as Go, Rust and TypeScript.
No independently authored or production-package qualification is claimed.
"""

from __future__ import annotations

from typing import List, Optional, Tuple as PyTuple

from ontos_core import Value
from ontos_data import (
    DataError,
    read_bool,
    read_int,
    read_list,
    read_map,
    read_set,
    read_text,
    recognize_int,
)

__all__ = ["wf", "conforms", "NAMESPACE", "MAX_RECURSION_DEPTH"]

#: The reserved descriptor-label namespace (spec §3). A descriptor combinator is
#: ``Tuple(Atom("horos/<ctor>"), args...)``.
NAMESPACE = "horos/"

#: The shared recursion-depth guard (horos-internal#37) — same logical trip depth as rs/go/ts.
MAX_RECURSION_DEPTH = 512

#: The horos-internal#18 atom-len/text-len bound cap: bounds must fit the signed 128-bit range.
#: Python ints are unbounded (like Go/TS), so the cap is applied explicitly —
#: a bound outside this window makes the descriptor non-wf / non-conforming,
#: byte-identically with the rs core (where ``read_int`` into i128 enforces it).
ATOM_LEN_BOUND_MAX = 2**127 - 1
ATOM_LEN_BOUND_MIN = -(2**127)

# ---------------------------------------------------------------------------
# descriptor head parsing
# ---------------------------------------------------------------------------


def _head(t: Value) -> Optional[PyTuple[str, List[Value]]]:
    """If ``t`` is ``Tuple(Atom("horos/<ctor>"), args...)`` with a valid-UTF-8
    label under the reserved namespace, return ``(<ctor>, args)``; else None."""
    if not t.is_tuple:
        return None
    items = t.items()
    if not items:
        return None
    first = items[0]
    if not first.is_atom:
        return None
    try:
        label = first.bytes().decode("utf-8")
    except UnicodeDecodeError:
        return None
    if not label.startswith(NAMESPACE):
        return None
    return label[len(NAMESPACE):], list(items[1:])


def _atom_bytes(v: Value) -> Optional[bytes]:
    return v.bytes() if v.is_atom else None


def _read_int_capped(v: Value) -> Optional[int]:
    """``ontos_data.read_int`` with the explicit horos-internal#18 i128 cap (rs enforces the
    same cap implicitly via its i128 materialization)."""
    try:
        n = read_int(v)
    except DataError:
        return None
    if n < ATOM_LEN_BOUND_MIN or n > ATOM_LEN_BOUND_MAX:
        return None
    return n


# ---------------------------------------------------------------------------
# data-encoded refinements (horos-internal#68) — explicit ASCII scans, mirroring rs/go/ts
# byte-for-byte (char-count semantics; the grammar literals are ASCII).
# ---------------------------------------------------------------------------

_ASCII_WS = {" ", "\t", "\n", "\r", "\x0b", "\x0c"}
_LOWER_HEX = set("0123456789abcdef")
_KEY_FIRST = set("abcdefghijklmnopqrstuvwxyz0123456789")
_KEY_REST = _KEY_FIRST | {".", "_", "-"}
_UUID_GROUPS = (8, 4, 4, 4, 12)


def _matches_key_grammar(s: str) -> bool:
    """``^[a-z0-9][a-z0-9._-]{0,63}$`` measured in characters (1–64 total)."""
    if not s or s[0] not in _KEY_FIRST:
        return False
    if len(s) > 64:
        return False
    return all(c in _KEY_REST for c in s[1:])


def _is_non_blank(s: str) -> bool:
    """At least one char outside the fixed ASCII whitespace set (NOT Unicode
    White_Space — parity with the other cores)."""
    return any(c not in _ASCII_WS for c in s)


def _matches_lowercase_uuid(s: str) -> bool:
    """``^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$``."""
    i = 0
    for gi, glen in enumerate(_UUID_GROUPS):
        for _ in range(glen):
            if i >= len(s) or s[i] not in _LOWER_HEX:
                return False
            i += 1
        if gi < len(_UUID_GROUPS) - 1:
            if i >= len(s) or s[i] != "-":
                return False
            i += 1
    return i == len(s)


# ---------------------------------------------------------------------------
# scope chains — cons-lists, innermost-first (the rs `Scope` semantics)
# ---------------------------------------------------------------------------
# env chain node:     ((name_bytes, body_descriptor), parent_or_None)
# pending chain node: (name_bytes, parent_or_None)


def _pending_contains(pending, name: bytes) -> bool:
    node = pending
    while node is not None:
        if node[0] == name:
            return True
        node = node[1]
    return False


def _env_lookup(env, name: bytes) -> Optional[Value]:
    """Innermost-first lookup (an inner ``rec`` shadows an outer binding)."""
    node = env
    while node is not None:
        entry_name, body = node[0]
        if entry_name == name:
            return body
        node = node[1]
    return None


# ---------------------------------------------------------------------------
# conformance — `v : T`
# ---------------------------------------------------------------------------


def conforms(v: Value, t: Value) -> bool:
    """Decide ``v : T``. Total and search-free: a non-conforming value AND a
    non-well-formed ``T`` both yield False; never raises."""
    # Soundness gate (spec §3 MUST): `v : T` presupposes `wf(T)`.
    return wf(t) and _conforms_in(v, t, None, None, 0)


def _conforms_in(v: Value, t: Value, env, pending, depth: int) -> bool:
    if depth > MAX_RECURSION_DEPTH:
        return False
    h = _head(t)
    if h is None:
        return False
    ctor, args = h
    n = len(args)

    # Top.
    if ctor == "any" and n == 0:
        return True

    # Atoms.
    if ctor == "atom" and n == 0:
        return v.is_atom
    if ctor == "atom-exact" and n == 1:
        vb, bb = _atom_bytes(v), _atom_bytes(args[0])
        return vb is not None and bb is not None and vb == bb
    if ctor == "atom-len" and n == 2:
        vb = _atom_bytes(v)
        lo, hi = _read_int_capped(args[0]), _read_int_capped(args[1])
        return vb is not None and lo is not None and hi is not None and lo <= len(vb) <= hi
    if ctor == "atom-in" and n == 1:
        vb = _atom_bytes(v)
        if vb is None:
            return False
        try:
            elems = read_set(args[0])
        except DataError:
            return False
        return any(_atom_bytes(e) == vb for e in elems)

    # Text refinements (horos-internal#68).
    if ctor == "text-len" and n == 2:
        try:
            s = read_text(v)
        except DataError:
            return False
        lo, hi = _read_int_capped(args[0]), _read_int_capped(args[1])
        return lo is not None and hi is not None and lo <= len(s) <= hi
    if ctor == "key-grammar" and n == 0:
        try:
            return _matches_key_grammar(read_text(v))
        except DataError:
            return False
    if ctor == "text-non-blank" and n == 0:
        try:
            return _is_non_blank(read_text(v))
        except DataError:
            return False
    if ctor == "lowercase-uuid" and n == 0:
        try:
            return _matches_lowercase_uuid(read_text(v))
        except DataError:
            return False

    # Scalars — delegate to the frozen ontos/data recognizers.
    if ctor == "scalar" and n == 1:
        kb = _atom_bytes(args[0])
        try:
            if kb == b"int":
                recognize_int(v)
                return True
            if kb == b"utf8-text":
                read_text(v)
                return True
            if kb == b"bool":
                read_bool(v)
                return True
        except DataError:
            return False
        return False

    # Tuples & compounds (descents: pending cleared).
    if ctor == "tuple":
        if not v.is_tuple:
            return False
        items = v.items()
        if len(items) != n:
            return False
        return all(
            _conforms_in(c, ct, env, None, depth + 1) for c, ct in zip(items, args)
        )
    if ctor == "seq" and n == 1:
        if not v.is_tuple:
            return False
        return all(_conforms_in(c, args[0], env, None, depth + 1) for c in v.items())
    if ctor == "compound" and n >= 1:
        lb = _atom_bytes(args[0])
        if not v.is_tuple or lb is None:
            return False
        items = v.items()
        ts = args[1:]
        if len(items) != len(ts) + 1 or _atom_bytes(items[0]) != lb:
            return False
        return all(
            _conforms_in(c, ct, env, None, depth + 1) for c, ct in zip(items[1:], ts)
        )

    # Structural collections.
    if ctor == "list-of" and n == 1:
        try:
            elems = read_list(v)
        except DataError:
            return False
        return all(_conforms_in(e, args[0], env, None, depth + 1) for e in elems)
    if ctor == "set-of" and n == 1:
        try:
            elems = read_set(v)
        except DataError:
            return False
        return all(_conforms_in(e, args[0], env, None, depth + 1) for e in elems)
    if ctor == "map-of" and n == 2:
        try:
            entries = read_map(v)
        except DataError:
            return False
        return all(
            _conforms_in(k, args[0], env, None, depth + 1)
            and _conforms_in(val, args[1], env, None, depth + 1)
            for k, val in entries
        )

    # Records.
    if ctor == "record" and n >= 1:
        return _conforms_record(v, args[0], args[1:], env, depth)

    # Composition (same `v`: pending threads through).
    if ctor == "union":
        return any(_conforms_in(v, ct, env, pending, depth + 1) for ct in args)
    if ctor == "and":
        return all(_conforms_in(v, ct, env, pending, depth + 1) for ct in args)
    if ctor == "not" and n == 1:
        return not _conforms_in(v, args[0], env, pending, depth + 1)

    # Singleton — ontos structural identity (defines no new equality).
    if ctor == "exact" and n == 1:
        return v == args[0]

    # Recursion.
    if ctor == "rec" and n == 2:
        nb = _atom_bytes(args[0])
        if nb is None:
            return False
        e = ((nb, args[1]), env)
        p = (nb, pending)
        return _conforms_in(v, args[1], e, p, depth + 1)
    if ctor == "ref" and n == 1:
        nb = _atom_bytes(args[0])
        if nb is None:
            return False
        if _pending_contains(pending, nb):
            return False  # unguarded at this node — would not terminate
        body = _env_lookup(env, nb)
        if body is None:
            return False  # unbound reference
        p = (nb, pending)
        return _conforms_in(v, body, env, p, depth + 1)

    return False  # unknown ctor or wrong arity


def _read_field(f: Value) -> Optional[PyTuple[str, bool, Value]]:
    """``Tuple(Atom("horos/field"), utf8-text(name), Atom(req|opt), T)``."""
    h = _head(f)
    if h is None or h[0] != "field" or len(h[1]) != 3:
        return None
    name_v, presence, ty = h[1]
    try:
        name = read_text(name_v)
    except DataError:
        return None
    pb = _atom_bytes(presence)
    if pb == b"req":
        required = True
    elif pb == b"opt":
        required = False
    else:
        return None
    return name, required, ty


def _conforms_record(v: Value, mode: Value, fields, env, depth: int) -> bool:
    mb = _atom_bytes(mode)
    if mb == b"open":
        open_mode = True
    elif mb == b"closed":
        open_mode = False
    else:
        return False
    try:
        entries = read_map(v)
    except DataError:
        return False
    present = {}
    for k, val in entries:
        try:
            key = read_text(k)
        except DataError:
            return False
        present[key] = val
    covered = 0
    for f in fields:
        rf = _read_field(f)
        if rf is None:
            return False
        name, required, ty = rf
        if name in present:
            covered += 1
            if not _conforms_in(present[name], ty, env, None, depth + 1):
                return False
        elif required:
            return False
    if not open_mode and covered != len(present):
        return False
    return True


# ---------------------------------------------------------------------------
# well-formedness — `wf(T)`
# ---------------------------------------------------------------------------


def wf(t: Value) -> bool:
    """Decide ``wf(T)``. Total and search-free; never raises."""
    return _wf_in(t, None, None, 0)


def _wf_in(t: Value, bound, pending, depth: int) -> bool:
    if depth > MAX_RECURSION_DEPTH:
        return False
    h = _head(t)
    if h is None:
        return False
    ctor, args = h
    n = len(args)

    if ctor in ("any", "atom") and n == 0:
        return True
    if ctor == "atom-exact" and n == 1:
        return args[0].is_atom
    if ctor in ("atom-len", "text-len") and n == 2:
        lo, hi = _read_int_capped(args[0]), _read_int_capped(args[1])
        return lo is not None and hi is not None and 0 <= lo <= hi
    if ctor == "atom-in" and n == 1:
        try:
            elems = read_set(args[0])
        except DataError:
            return False
        return all(e.is_atom for e in elems)
    if ctor in ("key-grammar", "text-non-blank", "lowercase-uuid") and n == 0:
        return True
    if ctor == "scalar" and n == 1:
        return _atom_bytes(args[0]) in (b"int", b"utf8-text", b"bool")

    # Descent combinators clear `pending`.
    if ctor == "tuple":
        return all(_wf_in(ct, bound, None, depth + 1) for ct in args)
    if ctor == "seq" and n == 1:
        return _wf_in(args[0], bound, None, depth + 1)
    if ctor == "compound" and n >= 1:
        return args[0].is_atom and all(
            _wf_in(ct, bound, None, depth + 1) for ct in args[1:]
        )
    if ctor in ("list-of", "set-of") and n == 1:
        return _wf_in(args[0], bound, None, depth + 1)
    if ctor == "map-of" and n == 2:
        return _wf_in(args[0], bound, None, depth + 1) and _wf_in(
            args[1], bound, None, depth + 1
        )
    if ctor == "record" and n >= 1:
        return _wf_record(args[0], args[1:], bound, depth)

    # Same-`v` combinators thread the guard through.
    if ctor in ("union", "and"):
        return all(_wf_in(ct, bound, pending, depth + 1) for ct in args)
    if ctor == "not" and n == 1:
        return _wf_in(args[0], bound, pending, depth + 1)
    if ctor == "exact" and n == 1:
        return True

    if ctor == "rec" and n == 2:
        nb = _atom_bytes(args[0])
        if nb is None:
            return False
        b = (nb, bound)
        p = (nb, pending)
        return _wf_in(args[1], b, p, depth + 1)
    if ctor == "ref" and n == 1:
        nb = _atom_bytes(args[0])
        if nb is None:
            return False
        return _pending_contains(bound, nb) and not _pending_contains(pending, nb)

    return False


def _wf_record(mode: Value, fields, bound, depth: int) -> bool:
    if _atom_bytes(mode) not in (b"open", b"closed"):
        return False
    names = set()
    for f in fields:
        rf = _read_field(f)
        if rf is None:
            return False
        name, _required, ty = rf
        if not _wf_in(ty, bound, None, depth + 1):
            return False
        if name in names:
            return False  # duplicate field name
        names.add(name)
    return True
