"""The diagnostic-location corpus matches its authored source.

The Python source has no diagnostic API, so this lane does not replay
``vectors/diagnose.json``. It checks that the corpus and its Go and Rust
package copies are exactly what ``vectors/diagnose.py`` encodes from the
authored cases, so the hex inputs cannot drift from the source.
"""

from __future__ import annotations

import subprocess
import sys
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]


class DiagnoseCorpusTest(unittest.TestCase):
    def test_corpus_matches_its_source(self) -> None:
        result = subprocess.run(
            [sys.executable, "-I", str(ROOT / "vectors" / "diagnose.py"), "--check"],
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)


if __name__ == "__main__":
    unittest.main()
