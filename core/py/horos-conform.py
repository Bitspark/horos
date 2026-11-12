"""Evaluate wf/conforms, and their diagnoses as JSON, over ontos-codec-v1 hex.

The Python lane's command-line face, with the same commands, output and JSON form as
the Go, Rust and TypeScript ``horos-conform`` tools, so conformance/run.py can compare
four implementations:

    horos-conform.py wf <descriptor> | conforms <descriptor> <value>
                   | diagnose-wf <descriptor> | diagnose <descriptor> <value>

``ONTOS_PY_ROOT`` names the ontos checkout whose core, codec and data packages it
imports, as the Python tests do. Like the Rust tool, it judges on a thread with a large
stack and a raised recursion limit, so deep finite inputs reach the logical depth bound
(docs/spec/horos.md, "Bounded checking and diagnostics") rather than the interpreter's.

Run:  ONTOS_PY_ROOT=<ontos v0.17.0 checkout> python3 -I core/py/horos-conform.py ...
"""

from __future__ import annotations

import json
import os
import re
import sys
import threading
from pathlib import Path

USAGE = (
    "usage: horos-conform wf <descriptor> | conforms <descriptor> <value> "
    "| diagnose-wf <descriptor> | diagnose <descriptor> <value>"
)
_HEX = re.compile(r"(?:[0-9a-f]{2})*")


def _setup() -> None:
    root = os.environ.get("ONTOS_PY_ROOT")
    if not root:
        raise SystemExit("Set ONTOS_PY_ROOT to an ontos v0.17.0 checkout")
    for sub in ("core/py", "codec/py", "data/py"):
        sys.path.insert(0, str(Path(root) / sub))
    sys.path.insert(0, str(Path(__file__).resolve().parent))


def _run(args: list) -> str:
    from ontos_codec import decode

    from horos import conforms, wf
    from horos.diagnose import diagnose, diagnose_wf

    def value(h: str):
        if not _HEX.fullmatch(h):
            raise ValueError("invalid lowercase hex")
        return decode(bytes.fromhex(h))

    def render(d) -> str:
        return json.dumps(d.to_json(), separators=(",", ":"), ensure_ascii=False)

    if len(args) == 2 and args[0] == "wf":
        return "true" if wf(value(args[1])) else "false"
    if len(args) == 3 and args[0] == "conforms":
        return "true" if conforms(value(args[2]), value(args[1])) else "false"
    if len(args) == 2 and args[0] == "diagnose-wf":
        return render(diagnose_wf(value(args[1])))
    if len(args) == 3 and args[0] == "diagnose":
        return render(diagnose(value(args[2]), value(args[1])))
    raise ValueError(USAGE)


def main() -> int:
    _setup()
    result: dict = {}

    def judge() -> None:
        try:
            result["out"] = _run(sys.argv[1:])
        except Exception as error:  # noqa: BLE001 - every failure is reported, as the other tools do
            result["err"] = str(error) or type(error).__name__

    sys.setrecursionlimit(1_000_000)
    threading.stack_size(512 * 1024 * 1024)
    thread = threading.Thread(target=judge)
    thread.start()
    thread.join()
    if "out" in result:
        print(result["out"])
        return 0
    print(result.get("err", "judgment thread failed"), file=sys.stderr)
    return 1


if __name__ == "__main__":
    sys.exit(main())
