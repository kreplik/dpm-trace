import os
import sys

import lit.formats


config.name = "dpm-trace"
config.test_format = lit.formats.ShTest()
config.suffixes = [".test"]
config.test_source_root = os.path.dirname(__file__)
config.test_exec_root = os.path.join(config.test_source_root, ".lit")

repo_root = os.path.dirname(config.test_source_root)

# %dpm is the dpm-trace binary. %python stays for the driver scripts, which are
# Python programs that drive the binary -- not a second implementation.
_binary = os.environ.get("DPM_TRACE_BIN", "").strip()
if not _binary:
    lit_config.fatal(
        "DPM_TRACE_BIN must point at the dpm-trace binary; build it with "
        "`go build -o /tmp/dpm-trace ./cmd/dpm-trace`")
dpm = os.path.abspath(os.path.expanduser(_binary))

config.substitutions.append(("%dpm", dpm))
config.substitutions.append(("%python", sys.executable))
config.substitutions.append(("%root", repo_root))
# The opt-in suites drive a sibling Daml package. It is resolved the same way
# tests/check-scaffolder-sync.py resolves it, so both follow one rename.
config.substitutions.append((
    "%daml-tests",
    os.environ.get("DPM_TRACE_DAML_TESTS_DIR", "").strip() or os.path.join(os.path.dirname(repo_root), "daml-contracts"),
))
config.substitutions.append(("%damlc", os.environ.get("DPM_TRACE_DAMLC", "daml")))
config.substitutions.append(("%daml", os.environ.get("DPM_TRACE_DAML", "daml")))

for key in (
    "HOME",
    # check-golden.py reads this itself, so lit must let it through or the
    # goldens silently run against Python while %dpm runs the binary.
    "DPM_TRACE_BIN",
    # A `go build -cover` binary warns on stderr when this is unset, which
    # corrupts output the suites compare. Forward it so coverage can be
    # measured across the subprocess runs.
    "GOCOVERDIR",
    "DPM_TRACE_DAML",
    "DPM_TRACE_DAMLC",
    "DPM_TRACE_CANTON_JAR",
    "DPM_TRACE_DAML_HELPER",
    "DPM_TRACE_PYTHON",
):
    if key in os.environ:
        config.environment[key] = os.environ[key]

if os.environ.get("DPM_TRACE_RUN_DAMLC_INSPECT") == "1":
    config.available_features.add("damlc-inspect")

if os.environ.get("DPM_TRACE_RUN_REAL_CANTON") == "1":
    config.available_features.add("real-canton")

if os.environ.get("DPM_TRACE_RUN_DAML_TEST") == "1":
    config.available_features.add("daml-test")
