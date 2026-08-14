"""Obtain the scaffolder's generated `itests/lit.cfg.py`.

The text used to be read straight out of `dpm_trace.cli.integration_lit_cfg_text`,
which pinned the checks to the Python implementation and left the Go build's
copy (internal/scaffold/templates/lit.cfg.py.tmpl) checked by nobody. Running
`dpm trace test --init` instead exercises whichever implementation is under
test, so both copies are covered by the same checks.
"""
from __future__ import annotations

import os
import subprocess
import sys
import tempfile
from pathlib import Path


def cli_command(repo_root: Path) -> list[str]:
    """The implementation under test, as in check-golden.py and lit.cfg.py."""
    binary_env = os.environ.get("DPM_TRACE_BIN")
    if binary_env:
        return binary_env.split()
    return [sys.executable, "-m", "dpm_trace.cli"]


def generated_lit_cfg(repo_root: Path) -> str:
    """Scaffold into a throwaway package and return its itests/lit.cfg.py."""
    with tempfile.TemporaryDirectory(prefix="dpm-trace-scaffold-") as work:
        package = Path(work)
        (package / "daml.yaml").write_text(
            "name: scaffold-check\nversion: 1.0.0\nsource: daml\n", encoding="utf-8"
        )
        (package / "daml").mkdir()
        env = {**os.environ, "PYTHONPATH": str(repo_root / "src")}
        result = subprocess.run(
            cli_command(repo_root) + ["test", str(package), "--init"],
            capture_output=True, text=True, env=env,
        )
        if result.returncode != 0:
            raise RuntimeError(f"--init failed (rc={result.returncode})\n{result.stderr}")
        cfg = package / "itests" / "lit.cfg.py"
        if not cfg.is_file():
            raise RuntimeError(f"--init wrote no {cfg}")
        return cfg.read_text(encoding="utf-8")
