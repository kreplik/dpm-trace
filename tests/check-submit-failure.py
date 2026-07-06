"""Simulates run_submit failure path: renders print_completion_trace output.

  (no flags)           compact output, text-match source search
  --full               verbose output, text-match source search
  --debug-info         compact output, simulated damlc-inspect source -> [debug-info] tag
  --full --debug-info  verbose output, simulated damlc-inspect source
"""
import argparse
import json
import sys
from pathlib import Path

root = Path(__file__).parent.parent
sys.path.insert(0, str(root / "src"))

from dpm_trace.cli import Color, SourceIndex, attach_log_matches, print_completion_trace

parser = argparse.ArgumentParser()
parser.add_argument("--full", action="store_true")
parser.add_argument("--debug-info", dest="debug_info", action="store_true")
args = parser.parse_args()

fixtures = root / "tests" / "fixtures"
daml_file = fixtures / "Sample.daml"

completion = json.loads((fixtures / "compare" / "completion-fail.json").read_text(encoding="utf-8"))

request = {
    "commandId": completion.get("commandId"),
    "commands": [{"CreateCommand": {"templateId": "205a59f940:Asset:Asset", "createArguments": {"owner": "Alice::1220a5", "quantity": 100}}}],
    "actAs": ["Alice::1220a5c9be4bbb245bd6257db398934eb7257f3b77176309fd724258b74e9d7b1f9f"],
}

log_args = argparse.Namespace(log_file=[str(fixtures / "canton-fixture.log")])
completion = attach_log_matches(log_args, completion)

if args.debug_info:
    source_index = SourceIndex()
    daml_lines = daml_file.read_text(encoding="utf-8").splitlines()
    source_index.files[str(daml_file)] = daml_lines
    source_index.module_files["Sample"] = [str(daml_file)]
    source_index.inspect_modules["Sample"] = daml_lines
else:
    source_index = SourceIndex(source_roots=[str(fixtures)])

print_completion_trace(
    completion,
    Color(enabled=False),
    source_index=source_index,
    request=request,
    compact=not args.full,
)
