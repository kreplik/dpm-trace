# Integration testing with `dpm trace`

`dpm trace test --integration` boots a real local Canton node, deploys your DAR,
runs a test suite against the live ledger, and tears the node down. Each test
submits a command and asserts on the transaction tree `dpm trace` prints — and a
failed live submission is mapped back to the contract source line, just like
unit-test failures.

`dpm trace` itself has no dependencies. The integration runner drives the suite
with two small external tools: **lit** (the test driver) and **FileCheck** (the
assertion matcher).

## Prerequisites

- A **Canton jar** — ships with the Daml SDK at
  `~/.daml/sdk/<version>/canton/canton.jar`.
- **lit** and **FileCheck** on your `PATH`:

```bash
pip install lit          # the lit test driver
pip install filecheck    # FileCheck, pure-Python build (installs as `filecheck`)
```

> The scaffolded tests call `FileCheck` (LLVM's name). If you installed the pip
> `filecheck` (lowercase), either symlink it —
> `ln -s "$(command -v filecheck)" /usr/local/bin/FileCheck` — or install LLVM's
> FileCheck instead: `brew install llvm` (macOS) / `sudo apt-get install -y llvm`
> (Debian/Ubuntu).
>
> FileCheck's pattern-matching language:
> <https://llvm.org/docs/CommandGuide/FileCheck.html>

## Set up and run

```bash
# 1. scaffold an integration suite (writes itests/ with a lit config + a sample test)
dpm-trace test . --init

# 2. run it against a managed local Canton
dpm-trace test . --integration itests \
  --canton-jar "$HOME/.daml/sdk/<version>/canton/canton.jar"
```

That's the whole loop — `--init` writes the config and a sample test, and
`--integration` boots Canton, deploys your DAR, runs the suite, and cleans up.
The exit code is non-zero if any test fails, so it gates CI.

## What a test looks like

Each `.test` file submits a command and checks the resulting trace. The runner
provides the substitutions `%ledger` (participant URL), `%alice`, `%bob`, `%dar`,
and `%dpm`/`%python`:

```
# RUN: ID=$(dpm-trace submit --submitter %ledger --act-as %alice \
#            --template '#yourpkg:Module:Template' --arg field=value) \
#   && dpm-trace "$ID" --submitter %ledger --read-as %alice | FileCheck %s

# CHECK: CREATE Module:Template
# CHECK: field: value
```

To assert that a submission *fails* and see the source it maps to, capture the
rejection with `dpm-trace submit --allow-fail` and trace it with
`dpm-trace --completion-file`.

## Notes

- lit + FileCheck are just the harness used here; `dpm trace` is not tied to
  them. You can call `dpm-trace submit` / `dpm-trace <update-id>` from any test
  framework and assert on the output however you like.
- See `daml-tests/itests/` (in the showcase repo) for a working suite, including
  a cross-participant test and a failed-submission test.
