"""Enforce one Daml SDK version across the repository.

examples/asset/daml.yaml pins the SDK, and the workflows read it from there.
Everything else that names an SDK version -- docs, the scaffolder and install
hints, test fixtures -- is a copy that can drift from it.
"""
from __future__ import annotations

import re
import subprocess
import sys
from pathlib import Path

PINNED_IN = "examples/asset/daml.yaml"

# Recorded output and captured ledger data name whatever produced them.
SKIP = re.compile(r"(^tests/golden/|\.json$|\.gif$|\.png$|_test\.go$|^tests/check-sdk-version\.py$)")

VERSION = re.compile(r"(?<![0-9.])3\.[0-9]+\.[0-9]+(?![0-9.])")


def main() -> int:
    if len(sys.argv) != 2:
        print("usage: check-sdk-version.py <repo-root>", file=sys.stderr)
        return 2
    root = Path(sys.argv[1]).resolve()

    match = re.search(r"^sdk-version:\s*(\S+)", (root / PINNED_IN).read_text(encoding="utf-8"), re.M)
    if not match:
        print(f"no sdk-version in {PINNED_IN}", file=sys.stderr)
        return 2
    pinned = match.group(1)

    files = subprocess.run(
        ["git", "-C", str(root), "ls-files"], check=True, capture_output=True, text=True
    ).stdout.splitlines()

    drift = []
    for rel in files:
        if SKIP.search(rel):
            continue
        try:
            text = (root / rel).read_text(encoding="utf-8")
        except (OSError, UnicodeDecodeError):
            continue
        for number, line in enumerate(text.splitlines(), 1):
            for found in VERSION.findall(line):
                if found != pinned:
                    drift.append(f"  {rel}:{number}: {found}")

    if drift:
        print(f"SDK version drift: {PINNED_IN} pins {pinned}, but these name another version:")
        print("\n".join(drift))
        return 1
    print(f"sdk-version checks passed: {pinned}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
