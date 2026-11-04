"""Reject candidates that do not bind the exact bytes consumed by the publishers."""
import base64
import copy
import hashlib
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import package

spec = importlib.util.spec_from_file_location("finalize_release", Path(__file__).with_name("finalize-release.py"))
finalize = importlib.util.module_from_spec(spec)
spec.loader.exec_module(finalize)


class ReceiptTest(unittest.TestCase):
    def test_artifact_binding(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            (root / "vectors").mkdir()
            (root / "vectors/conformance.json").write_bytes(b"oracle")
            entries = []
            for registry, name, suffix in [("npm", "@bitspark/horos", "tgz"), ("crates.io", "bitspark-horos", "crate")]:
                file = f"bitspark-horos-0.9.0.{suffix}"
                content = suffix.encode()
                (root / file).write_bytes(content)
                entries.append({"registry": registry, "name": name, "version": "0.9.0", "file": file,
                                "sha256": hashlib.sha256(content).hexdigest(), "size": len(content),
                                "integrity": "sha512-" + base64.b64encode(hashlib.sha512(content).digest()).decode()})
            original = {"schema": 1, "version": "0.9.0", "mode": "rehearse", "source": "source", "tree": "tree",
                        "tags": ["v0.9.0", "core/go/v0.9.0"], "oracle_sha256": package.digest(root / "vectors/conformance.json"), "packages": entries}

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


class FinalizationTest(unittest.TestCase):
    def test_incomplete_release_recovery(self):
        accepted = {"source": "source", "version": "0.9.0", "oracle_sha256": "oracle",
                    "packages": [{"file": "npm.tgz"}, {"file": "cargo.crate"}]}
        verified = {"source": "source", "version": "0.9.0", "oracle_sha256": "oracle", "mode": "published",
                    "anonymous_consumers": ["npm", "crates.io", "Go proxy"], "go_origin": {"Hash": "source"},
                    "ci": "https://github.com/Bitspark/horos/actions/runs/1", "run_id": "2"}
        for scenario in ("complete", "missing", "source", "version", "oracle_sha256", "mode", "consumers", "artifact"):
            with self.subTest(scenario=scenario), tempfile.TemporaryDirectory() as temp:
                directory = Path(temp)
                remote = {name: name.encode() for name in ("npm.tgz", "cargo.crate", "candidate.json")}
                for name, data in remote.items():
                    (directory / name).write_bytes(data)
                proof_name = "published-verification.json"
                (directory / proof_name).write_text(json.dumps(verified))
                original = dict(verified, run_id="1")
                if scenario in ("source", "version", "oracle_sha256", "mode"):
                    original[scenario] = "wrong"
                if scenario == "consumers": original["anonymous_consumers"] = ["npm", "crates.io"]
                if scenario != "missing": remote[proof_name] = json.dumps(original).encode()
                if scenario == "artifact": remote["npm.tgz"] = b"different"
                uploads = []

                def gh(args, **kwargs):
                    if args[2] == "download":
                        destination = Path(args[args.index("--dir") + 1])
                        selected = {proof_name} if "--pattern" in args else set(remote)
                        for name in selected:
                            (destination / name).write_bytes(remote[name])
                    elif args[2] == "upload":
                        self.assertNotIn("--clobber", args)
                        uploaded = Path(args[4])
                        self.assertEqual(uploaded.name, proof_name)
                        self.assertNotIn(uploaded.name, remote)
                        uploads.append(uploaded.name)
                        remote[uploaded.name] = uploaded.read_bytes()
                    else:
                        self.fail(f"unexpected provider action: {args}")

                with patch.object(finalize.subprocess, "run", side_effect=gh):
                    if scenario in ("complete", "missing"):
                        finalize.preserve_existing(directory, accepted, "Bitspark/horos")
                        self.assertEqual(uploads, [proof_name] if scenario == "missing" else [])
                        self.assertEqual(json.loads(remote[proof_name])["run_id"], "2" if scenario == "missing" else "1")
                    else:
                        with self.assertRaises(AssertionError):
                            finalize.preserve_existing(directory, accepted, "Bitspark/horos")
                        self.assertEqual(uploads, [])


if __name__ == "__main__":
    unittest.main()
