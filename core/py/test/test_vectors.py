"""Oracle replay — the py lane's truth anchor.

Replays every case in ``vectors/conformance.json`` (the shared, hand-authored,
byte-pinned oracle the tri-core replays) through the py recognizer: each
descriptor and value is decoded from ontos-codec-v1 hex via ontos's OWN py
codec, then ``wf``/``conforms`` must return exactly the oracle's verdicts.

Locating ontos: set ``ONTOS_PY_ROOT`` to an ontos checkout at the pinned
coordinate: the ontos version the rs/go/ts cores link, as core/go/go.mod
names it (CI reads it from there, horos-internal#243). No
repo-escaping relative paths; the env var is the only discovery mechanism.

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
    raise SystemExit(
        "ONTOS_PY_ROOT is not set. Point it at an ontos checkout at the pinned "
        "coordinate (the github.com/bitspark/ontos version in core/go/go.mod); "
        "its core/py, codec/py, and data/py are imported."
    )
for sub in ("core/py", "codec/py", "data/py"):
    p = str(Path(_ONTOS) / sub)
    if p not in sys.path:
        sys.path.insert(0, p)

_HERE = Path(__file__).resolve()
sys.path.insert(0, str(_HERE.parents[1]))  # core/py → import horos

from ontos_codec import decode  # noqa: E402
from horos import conforms, wf  # noqa: E402

_ORACLE = _HERE.parents[3] / "vectors" / "conformance.json"


class TestVectorsReplay(unittest.TestCase):
    """Every oracle verdict, byte-identical with the tri-core."""

    def test_replay_all_cases(self) -> None:
        with open(_ORACLE, encoding="utf-8") as f:
            oracle = json.load(f)
        cases = oracle["cases"]
        self.assertGreater(len(cases), 0, "oracle is empty?")
        checked_wf = 0
        checked_values = 0
        for case in cases:
            name = case["name"]
            descriptor = decode(bytes.fromhex(case["descriptor"]))
            self.assertEqual(
                wf(descriptor), case["wf"], f"wf mismatch for case {name!r}"
            )
            checked_wf += 1
            for i, vc in enumerate(case.get("values", [])):
                value = decode(bytes.fromhex(vc["value"]))
                note = vc.get("note", f"#{i}")
                self.assertEqual(
                    conforms(value, descriptor),
                    vc["conforms"],
                    f"conforms mismatch for case {name!r}, value {note!r}",
                )
                checked_values += 1
        print(
            f"py replay: {checked_wf} wf verdicts + {checked_values} conforms "
            f"verdicts, all matching the oracle"
        )


if __name__ == "__main__":
    unittest.main()
