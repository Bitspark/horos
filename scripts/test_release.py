"""Reject candidates that do not bind the exact bytes consumed by the publishers."""
import base64
import copy
import hashlib
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import package


class ReceiptTest(unittest.TestCase):
    def test_artifact_binding(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            (root / "vectors").mkdir()
            (root / "vectors/conformance.json").write_bytes(b"oracle")
            entries = []
            for registry, name, suffix in [("npm", "@bitspark/horos", "tgz"), ("crates.io", "bitspark-horos", "crate")]:
                file = f"bitspark-horos-0.6.0.{suffix}"
                content = suffix.encode()
                (root / file).write_bytes(content)
                entries.append({"registry": registry, "name": name, "version": "0.6.0", "file": file,
                                "sha256": hashlib.sha256(content).hexdigest(), "size": len(content),
                                "integrity": "sha512-" + base64.b64encode(hashlib.sha512(content).digest()).decode()})
            original = {"schema": 1, "version": "0.6.0", "mode": "rehearse", "source": "source", "tree": "tree",
                        "tags": ["v0.6.0", "core/go/v0.6.0"], "oracle_sha256": package.digest(root / "vectors/conformance.json"), "packages": entries}

            def check(data):
                (root / "candidate.json").write_text(json.dumps(data))
                return package.receipt(root)

            with patch.object(package, "ROOT", root), patch.object(package, "run", side_effect=lambda args: "tree" if args[-1] == "HEAD^{tree}" else "source"):
                self.assertEqual(check(original), original)
                for change in ("missing", "rename", "duplicate", "version", "schema", "tree", "tags", "size", "integrity"):
                    data = copy.deepcopy(original)
                    if change == "missing": data["packages"].pop()
                    elif change == "rename": data["packages"][0]["file"] = "unhashed.tgz"
                    elif change == "duplicate": data["packages"][1] = data["packages"][0]
                    elif change in ("size", "integrity", "version"): data["packages"][0][change] = "wrong"
                    else: data[change] = "wrong"
                    with self.subTest(change=change), self.assertRaises((AssertionError, KeyError)):
                        check(data)
                (root / entries[0]["file"]).write_bytes(b"tampered")
                with self.assertRaises(AssertionError): check(original)


if __name__ == "__main__":
    unittest.main()
