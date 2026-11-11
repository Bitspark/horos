"""The Python command-line face answers in the other tools' contract.

conformance/run.py compares its output with the Go, Rust and TypeScript tools
judgment by judgment; this checks the contract itself on a few inputs: the four
commands, their output forms, and a failure on malformed input.

Run:  ONTOS_PY_ROOT=<ontos checkout> python -m unittest discover core/py/test
"""

from __future__ import annotations

import json
import os
import subprocess
import sys
import unittest
from pathlib import Path

CLI = Path(__file__).resolve().parents[1] / "horos-conform.py"

# The ADR 0011 reproducer and the value it was reported with, and its false-positive twin.
REPRODUCER = (
    "01030009686f726f732f72656300014d0103000b686f726f732f756e696f6e0102000b686f726f732f6578616374000161"
    "01030009686f726f732f72656300014c0103000b686f726f732f7475706c6501020009686f726f732f72656600014d0103"
    "0009686f726f732f72656300014d0103000b686f726f732f756e696f6e0102000b686f726f732f657861637400016201"
    "02000b686f726f732f7475706c6501020009686f726f732f72656600014c"
)
REPORTED = "010200016101010102000161000162"
TWIN = "010200016101010102000162000162"


def run(*args: str) -> subprocess.CompletedProcess:
    if not os.environ.get("ONTOS_PY_ROOT"):
        raise SystemExit("ONTOS_PY_ROOT is not set; see core/py/test/test_vectors.py.")
    return subprocess.run([sys.executable, "-I", str(CLI), *args], capture_output=True, text=True, check=False)


class CommandLineContract(unittest.TestCase):
    def test_booleans_print_true_or_false(self) -> None:
        self.assertEqual(run("wf", REPRODUCER).stdout, "true\n")
        self.assertEqual(run("conforms", REPRODUCER, REPORTED).stdout, "true\n")
        self.assertEqual(run("conforms", REPRODUCER, TWIN).stdout, "false\n")

    def test_diagnoses_print_the_contract_json(self) -> None:
        self.assertEqual(run("diagnose-wf", REPRODUCER).stdout, '{"verdict":"conforms"}\n')
        out = run("diagnose", REPRODUCER, TWIN).stdout
        self.assertEqual(
            json.loads(out),
            {
                "verdict": "mismatch",
                "location": {
                    "path": [{"kind": "item", "index": 1}, {"kind": "item", "index": 0}, {"kind": "item", "index": 0}],
                    "at": [2],
                    "reason": "no-alternative",
                },
            },
        )
        self.assertNotIn(" ", out, "compact JSON, as the other tools print")

    def test_malformed_input_fails_with_a_message(self) -> None:
        for args in (("wf", "0G"), ("wf", "01"), ("conforms", REPRODUCER), ("frob", REPRODUCER)):
            result = run(*args)
            self.assertEqual(result.returncode, 1, args)
            self.assertEqual(result.stdout, "", args)
            self.assertTrue(result.stderr.strip(), args)


if __name__ == "__main__":
    unittest.main()
