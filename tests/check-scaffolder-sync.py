"""Enforce that the scaffolder's generated `lit.cfg.py` stays in sync with the
canonical sibling `daml-contracts/itests/lit.cfg.py`.

The scaffolder embeds a copy of `itests/lit.cfg.py`. AGENTS.md requires it stays
in sync with the sibling, but that file lives in a separate repo, so drift is
otherwise silent.

The text is obtained by running `dpm trace test --init`, so this checks whichever
implementation DPM_TRACE_BIN selects rather than the Python source alone.

This check diffs the embedded text against the sibling canonical file when it
is available. If the sibling package is not present (e.g. CI that only checks
out `dpm-trace`), the check exits 0 with a SKIP notice so it never blocks
unrelated pipelines; where the sibling is checked out (the canonical Walnut
workspace), drift fails the suite.
"""
from __future__ import annotations

import os
import sys
from pathlib import Path


def _norm(text: str) -> str:
    return text.replace("\r\n", "\n").rstrip() + "\n"


def main() -> int:
    if len(sys.argv) != 2:
        print("usage: check-scaffolder-sync.py <repo-root>", file=sys.stderr)
        return 2
    repo_root = Path(sys.argv[1]).resolve()

    env_sibling = os.environ.get("DPM_TRACE_DAML_TESTS_DIR", "").strip()
    sibling = Path(env_sibling).resolve() if env_sibling else repo_root.parent / "daml-contracts"
    cfg = sibling / "itests" / "lit.cfg.py"
    if not cfg.is_file():
        print(
            f"scaffolder-sync checks skipped: sibling daml-contracts/itests/lit.cfg.py "
            f"not found at {cfg} (set DPM_TRACE_DAML_TESTS_DIR to enable)."
        )
        return 0

    sys.path.insert(0, str(Path(__file__).resolve().parent))
    from scaffold_output import generated_lit_cfg  # noqa: E402

    embedded = _norm(generated_lit_cfg(repo_root))
    canonical = _norm(cfg.read_text(encoding="utf-8"))
    if embedded != canonical:
        print(
            "scaffolder-sync checks FAILED:\n"
            f"  - the scaffolded lit.cfg.py drifts from {cfg} (canonical). "
            "Update the scaffolder template to match the sibling file."
        )
        return 1
    print("scaffolder-sync checks passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
