"""Obtain the scaffolder's generated `itests/lit.cfg.py`.

Obtained by running `dpm trace test --init`, so the checks cover the template
that actually ships (internal/scaffold/templates/lit.cfg.py.tmpl) rather than a
separate copy.
"""
from __future__ import annotations

import os
import subprocess
import sys
import tempfile
from pathlib import Path


def cli_command(repo_root: Path) -> list[str]:
    """The implementation under test, as in check-golden.py and lit.cfg.py."""
    binary_env = os.environ.get("DPM_TRACE_BIN", "").strip()
    if not binary_env:
        raise SystemExit("DPM_TRACE_BIN must point at the dpm-trace binary")
    return binary_env.split()


def generated_lit_cfg(repo_root: Path) -> str:
    """Scaffold into a throwaway package and return its itests/lit.cfg.py."""
    with tempfile.TemporaryDirectory(prefix="dpm-trace-scaffold-") as work:
        package = Path(work)
        (package / "daml.yaml").write_text(
            "name: scaffold-check\nversion: 1.0.0\nsource: daml\n", encoding="utf-8"
        )
        (package / "daml").mkdir()
        result = subprocess.run(
            cli_command(repo_root) + ["test", str(package), "--init"],
            capture_output=True, text=True,
        )
        if result.returncode != 0:
            raise RuntimeError(f"--init failed (rc={result.returncode})\n{result.stderr}")
        cfg = package / "itests" / "lit.cfg.py"
        if not cfg.is_file():
            raise RuntimeError(f"--init wrote no {cfg}")
        return cfg.read_text(encoding="utf-8")
