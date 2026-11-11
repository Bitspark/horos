"""The Python lane's diagnostics: ADR 0008 verdicts and ADR 0009 locations.

``vectors/diagnose.json`` is replayed in full: every ``diagnose_wf`` and
``diagnose`` result must equal the authored JSON form, location included. The
original oracle and the integer-bounds oracle are swept through the diagnostic
API: every completed diagnosis must equal the oracle's authored judgment, and the
only exhausted judgment is the oracle's authored over-the-cap case.

Run:  ONTOS_PY_ROOT=<ontos checkout> python -m unittest discover core/py/test
"""

from __future__ import annotations

import json
import os
import sys
import unittest
from pathlib import Path

_ONTOS = os.environ.get("ONTOS_PY_ROOT")
if not _ONTOS:
    raise SystemExit("ONTOS_PY_ROOT is not set; see core/py/test/test_vectors.py.")
for sub in ("core/py", "codec/py", "data/py"):
    p = str(Path(_ONTOS) / sub)
    if p not in sys.path:
        sys.path.insert(0, p)

_HERE = Path(__file__).resolve()
sys.path.insert(0, str(_HERE.parents[1]))  # core/py → import horos

from ontos_codec import decode  # noqa: E402
from ontos_core import atom, tuple_  # noqa: E402
from ontos_data import encode_int, encode_text  # noqa: E402
from horos import MAX_RECURSION_DEPTH  # noqa: E402
from horos.diagnose import (  # noqa: E402
    Diagnosis,
    Location,
    Step,
    checking_profile,
    diagnose,
    diagnose_wf,
)

_VECTORS = _HERE.parents[3] / "vectors"


def _decode(h: str):
    return decode(bytes.fromhex(h))


class DiagnoseCorpusReplay(unittest.TestCase):
    """vectors/diagnose.json, every expected diagnosis, location included."""

    def test_replay_the_location_corpus(self) -> None:
        doc = json.loads((_VECTORS / "diagnose.json").read_text(encoding="utf-8"))
        self.assertEqual(doc["version"], "horos-diagnose-v1")
        disagreements = []
        checked = 0
        for case in doc["cases"]:
            t = _decode(case["descriptor"])
            got = diagnose_wf(t).to_json()
            if got != case["wf"]:
                disagreements.append({"case": case["name"], "diagnose_wf": got, "expected": case["wf"]})
            checked += 1
            for vc in case["values"]:
                got = diagnose(_decode(vc["value"]), t).to_json()
                if got != vc["diagnosis"]:
                    disagreements.append(
                        {"case": case["name"], "value": vc["note"], "diagnose": got, "expected": vc["diagnosis"]}
                    )
                checked += 1
        self.assertEqual(disagreements, [], json.dumps(disagreements, indent=1))
        print(f"py diagnose replay: {checked} diagnoses over {len(doc['cases'])} cases, all matching")


class OracleSweep(unittest.TestCase):
    """ADR 0008: on completed outcomes the diagnosis agrees with the authored judgment."""

    # The original oracle's case whose value its notes author as tripping the depth cap.
    OVER_THE_CAP = {("conformance.json", "depth-171-over-cap-false")}

    def test_completed_diagnoses_agree_with_the_oracles(self) -> None:
        exhausted = set()
        completed = 0
        for name in ("conformance.json", "integer-bounds.json"):
            doc = json.loads((_VECTORS / name).read_text(encoding="utf-8"))
            for case in doc["cases"]:
                t = _decode(case["descriptor"])
                wf_bool = diagnose_wf(t).completed_bool()
                self.assertIsNotNone(wf_bool, f"{name} {case['name']}: diagnose_wf exhausted")
                self.assertEqual(wf_bool, case["wf"], f"{name} {case['name']}: diagnose_wf")
                completed += 1
                for vc in case["values"]:
                    d = diagnose(_decode(vc["value"]), t)
                    b = d.completed_bool()
                    if b is None:
                        exhausted.add((name, case["name"]))
                        continue
                    self.assertEqual(b, vc["conforms"], f"{name} {case['name']} / {vc['note']}: {d.to_json()}")
                    completed += 1
        self.assertEqual(exhausted, self.OVER_THE_CAP)
        print(f"py diagnose sweep: {completed} completed diagnoses agree with the oracles; 1 exhausted")


def _a(s: str):
    return atom(s.encode("utf-8"))


def _d(ctor: str, *args):
    return tuple_([_a("horos/" + ctor), *args])


def _deep_cons(n: int):
    v = _a("[]")
    for _ in range(n):
        v = tuple_([encode_int(1), v])
    return v


def _nest(ctor: str, n: int, inner):
    for _ in range(n):
        inner = _d(ctor, inner)
    return inner


class ReadingsBeyondTheCorpus(unittest.TestCase):
    """Precedence the contract implies but vectors/diagnose.json does not exercise.

    Authored from docs/spec/horos.md ("among those that decided the verdict") and
    ADR 0008/0009. FLAT's union for cons cell k is entered 1 + 3k levels below its
    rec, so a 171-cell value exhausts it; DEEP spends five levels per cell.
    """

    @classmethod
    def setUpClass(cls) -> None:
        cls.INT = _d("scalar", _a("int"))
        cls.FLAT = _d("rec", _a("L"), _d("union", _d("exact", _a("[]")), _d("tuple", cls.INT, _d("ref", _a("L")))))
        cls.DEEP = _d(
            "rec",
            _a("L"),
            _d("union", _d("exact", _a("[]")), _d("tuple", cls.INT, _d("not", _d("not", _d("ref", _a("L")))))),
        )

    def test_a_mismatching_item_decides_over_an_exhausted_sibling(self) -> None:
        d = diagnose(tuple_([_deep_cons(171), encode_text("x")]), _d("tuple", self.FLAT, self.INT))
        self.assertEqual(d, Diagnosis("mismatch", Location((Step("item", 1),), (2,), "refused")))

    def test_a_mismatching_field_decides_over_an_exhausted_field(self) -> None:
        t = _d(
            "record",
            _a("closed"),
            _d("field", encode_text("a"), _a("req"), self.FLAT),
            _d("field", encode_text("b"), _a("req"), self.INT),
        )
        v = tuple_([_a("map"), tuple_([encode_text("a"), _deep_cons(171)]), tuple_([encode_text("b"), encode_text("x")])])
        self.assertEqual(diagnose(v, t), Diagnosis("mismatch", Location((Step("field", name="b"),), (3, 3), "refused")))

    def test_an_exhausted_union_takes_its_first_exhausted_alternative(self) -> None:
        # DEEP exhausts at cell 102, FLAT at cell 170: item order decides, not path length.
        d = diagnose(_deep_cons(171), _d("union", self.DEEP, self.FLAT))
        self.assertEqual(d.verdict, "exhausted")
        self.assertEqual(d.location, Location((Step("item", 1),) * 102, (1, 2, 1), "limit"))

    def test_an_exhausted_child_decides_over_an_earlier_malformed_child(self) -> None:
        d = diagnose_wf(_d("union", _a("x"), _nest("not", 600, _d("any"))))
        self.assertEqual(d, Diagnosis("exhausted", Location((), (2,) + (1,) * 512, "limit"), "recursion_depth"))


class DiagnosisApi(unittest.TestCase):
    def test_profile_states_the_limits(self) -> None:
        p = checking_profile()
        self.assertEqual(p.max_recursion_depth, MAX_RECURSION_DEPTH)
        self.assertEqual((p.bound_min, p.bound_max), (-(2**127), 2**127 - 1))

    def test_completed_bool(self) -> None:
        loc = Location((), (), "refused")
        self.assertIs(Diagnosis("conforms").completed_bool(), True)
        self.assertIs(Diagnosis("mismatch", loc).completed_bool(), False)
        self.assertIs(Diagnosis("not_well_formed", loc).completed_bool(), False)
        exhausted = Diagnosis("exhausted", loc, "recursion_depth")
        self.assertIsNone(exhausted.completed_bool())
        self.assertFalse(exhausted.complete)

    def test_location_values(self) -> None:
        d = diagnose(tuple_([encode_int(1)]), _d("tuple", _d("atom")))
        self.assertEqual(d, Diagnosis("mismatch", Location((Step("item", 0),), (1,), "refused")))
        self.assertEqual(diagnose(_a("x"), _d("any")), Diagnosis("conforms"))

    def test_not_a_descriptor_is_diagnosed_not_raised(self) -> None:
        for t in (_a("x"), tuple_([]), tuple_([encode_int(1)]), tuple_([atom(b"horos/\xff")])):
            self.assertEqual(diagnose(_a("x"), t).verdict, "not_well_formed")
            self.assertEqual(diagnose_wf(t).location, Location((), (), "not-descriptor"))


if __name__ == "__main__":
    unittest.main()
