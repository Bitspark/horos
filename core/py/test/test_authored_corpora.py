"""The authored corpora match their sources.

``test_diagnose.py`` replays ``vectors/diagnose.json``. This test checks that
each authored corpus (``vectors/diagnose.json`` and
``vectors/integer-bounds.json``) and its Go and Rust package copies are exactly
what its script encodes from the authored cases, so the hex inputs cannot drift
from the source.
"""

from __future__ import annotations

import subprocess
import sys
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]


class AuthoredCorpusTest(unittest.TestCase):
    def test_corpora_match_their_sources(self) -> None:
        for script in ("diagnose.py", "integer-bounds.py"):
            result = subprocess.run(
                [sys.executable, "-I", str(ROOT / "vectors" / script), "--check"],
                capture_output=True,
                text=True,
                check=False,
            )
            self.assertEqual(result.returncode, 0, script + ": " + result.stdout + result.stderr)


if __name__ == "__main__":
    unittest.main()
