"""Public source/metadata contract; no network or credential access."""
from pathlib import Path
import hashlib
import json
import re
import subprocess
import tomllib

ROOT = Path(__file__).resolve().parent.parent
files = subprocess.check_output(["git", "ls-files", "-z"], cwd=ROOT).decode().split("\0")
files = [p for p in files if p]
assert files, "no tracked public source"
for name in files:
    assert not re.search(r"(^|/)(\.bb-board|\.agents|\.claude|substrate|research-docs|AGENTS\.md|CLAUDE\.md)(/|$)", name), name
    assert not re.search(r"(semantics|witness|verify.guard|linked.versions|wire)\.(go|rs|ts|json)$", name), name
    if name.endswith((".ts", ".go", ".rs", ".py", ".json", ".toml", ".yml", ".md", ".sh")):
        text = (ROOT / name).read_text(encoding="utf-8")
        # Test code may assert absence of old entrypoints; no private dependency or URL.
        for pattern in (r"npm\.pkg\.github\.com", r"@bitspark/logos", r"github\.com/[Bb]itspark/(logos|atlas|design)(?:/|\b)", r"vars\.CI_RUNNER", r"uses:\s*Bitspark/", r"seat:[a-z]", r"Acted-by:"):
            if name == "scripts/verify-public.py":
                continue
            assert not re.search(pattern, text), (name, pattern)
        if name.startswith(".github/workflows/"):
            for runner in re.findall(r"runs-on:\s*(.+)", text):
                assert runner.strip() == "ubuntu-latest", (name, runner)

pkg = json.loads((ROOT / "core/ts/package.json").read_text())
assert pkg["version"] == "0.6.0" and pkg["license"] == "Apache-2.0"
assert set(pkg["exports"]) == {".", "./diagnose", "./package.json"}
assert pkg["dependencies"] == {"@bitspark/ontos-" + p: "0.15.0" for p in ("core", "codec", "data")}
assert pkg["publishConfig"]["registry"] == "https://registry.npmjs.org/"
crate = tomllib.loads((ROOT / "core/rs/Cargo.toml").read_text())
assert crate["package"]["name"] == "bitspark-horos" and crate["package"]["version"] == "0.6.0"
assert crate["lib"]["name"] == "horos"
assert all(d.get("version") == "=0.15.0" and "git" not in d for d in crate["dependencies"].values())
assert "github.com/bitspark/ontos v0.15.0" in (ROOT / "core/go/go.mod").read_text()
assert "logos" not in (ROOT / "core/go/go.mod").read_text().lower()

oracle = (ROOT / "vectors/conformance.json").read_bytes()
expected = (ROOT / "vectors/conformance.sha256").read_text().strip()
assert hashlib.sha256(oracle).hexdigest() == expected, "original oracle changed"
for lane in ("go", "rs"):
    assert (ROOT / f"core/{lane}/testdata/conformance.json").read_bytes() == oracle

for name in files:
    if not name.endswith(".md"):
        continue
    for link in re.findall(r"\[[^\]]*\]\(([^)]+)\)", (ROOT / name).read_text()):
        target = link.split("#", 1)[0]
        if not target or re.match(r"[a-z]+:", target):
            continue
        assert (ROOT / name).parent.joinpath(target).is_file(), (name, target)
print(f"Public source, metadata, links and original oracle checked: {len(files)} files")
