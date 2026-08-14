"""Enforce that `examples/asset` stays in sync with the canonical sibling package.

The Milestone 1 acceptance criteria require the representative Daml examples to
be *included* in this repo, so `examples/asset` is a copy of the sibling
`daml-contracts` package. The sibling is the canonical one -- it is what the
integration suite exercises against a real Canton -- so the copy here must not
drift from it.

Like tests/check-scaffolder-sync.py, this diffs against the sibling when it is
checked out and exits 0 with a SKIP notice when it is not, so a checkout of
dpm-trace alone never fails on a file it cannot see.
"""
from __future__ import annotations

import os
import sys
from pathlib import Path

# Sibling path -> path under examples/asset.
MIRRORED = {
    "daml.yaml": "daml.yaml",
    "daml/Asset.daml": "daml/Asset.daml",
}


def _norm(text: str) -> str:
    return text.replace("\r\n", "\n").rstrip() + "\n"


def main() -> int:
    if len(sys.argv) != 2:
        print("usage: check-examples-sync.py <repo-root>", file=sys.stderr)
        return 2
    repo_root = Path(sys.argv[1]).resolve()

    env_sibling = os.environ.get("DPM_TRACE_DAML_TESTS_DIR", "").strip()
    sibling = Path(env_sibling).resolve() if env_sibling else repo_root.parent / "daml-contracts"
    if not (sibling / "daml.yaml").is_file():
        print(
            f"examples-sync checks skipped: sibling package not found at {sibling} "
            "(set DPM_TRACE_DAML_TESTS_DIR to enable)."
        )
        return 0

    errors: list[str] = []
    for relative, mirrored in MIRRORED.items():
        canonical = sibling / relative
        local = repo_root / "examples" / "asset" / mirrored
        if not canonical.is_file():
            errors.append(f"canonical file missing: {canonical}")
            continue
        if not local.is_file():
            errors.append(f"example file missing: {local}")
            continue
        if _norm(canonical.read_text(encoding="utf-8")) != _norm(local.read_text(encoding="utf-8")):
            errors.append(f"{local} drifts from {canonical} (canonical). Copy the sibling file over.")

    if errors:
        print("examples-sync checks FAILED:")
        for err in errors:
            print(f"  - {err}")
        return 1
    print("examples-sync checks passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
