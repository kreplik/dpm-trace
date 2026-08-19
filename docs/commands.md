# Command reference

Full flags and workflows for `dpm trace`. The [README](../README.md) covers
installation and a quick tour; this is the detail behind it.

## Configuration

An optional per-project config saves repeating connection flags:

```bash
cp .dpm-trace.example.json .dpm-trace.json
```

```json
{
  "ledgerUrl": "http://localhost:<json-ledger-api-port>",
  "readAs": "<party-id>",
  "darPaths": ["./path/to/app.dar"]
}
```

Do not commit `.dpm-trace.json`.

## trace

Inspect a committed transaction:

```bash
dpm trace <update-id>
```

With explicit participant context:

```bash
dpm trace <update-id> \
  --submitter http://localhost:<json-ledger-api-port> \
  --read-as '<party-id>' \
  --access-token-file ./token.txt
```

The bearer token can also come from `--token`, `DPM_TRACE_TOKEN` or
`DPM_TRACE_TOKEN_FILE`.

Export the artifact for later, or open the stepper:

```bash
dpm trace <update-id> --export trace.json
dpm trace <update-id> --visualize
```

Because a committed update reaches other participants asynchronously, `--wait
<seconds>` retries until it becomes visible.

### Failed submissions

A failed command never becomes a transaction, so there is no update id. Read the
completion instead, by command id:

```bash
dpm trace --command-id <command-id> \
  --submitter http://localhost:<json-ledger-api-port> \
  --act-as '<party-id>' \
  --log-file /tmp/canton-participant.log
```

…or from captured JSON, which is the CI-friendly path:

```bash
dpm trace --completion-file completion.json \
  --log-file /tmp/canton-participant.log
```

With a local Daml project available, the failure is resolved to the contract
line and column. Passing `--dar` makes `dpm trace` confirm the failure text
exists in the compiled package via `damlc inspect` before matching local
sources, rather than grepping files:

```bash
dpm trace --completion-file completion.json \
  --daml-yaml <path-to-daml-project>/daml.yaml \
  --dar <path-to-daml-project>/.daml/dist/app.dar \
  --damlc daml
```

The output carries a `Source diagnostics` block with `file:line:column` and a
caret under the matching code.

### Event kinds

The tree renders create, exercise, archive and reassignment events. A
reassignment shows as `UNASSIGN` on the source synchronizer and `ASSIGN` on the
target, each with the source/target synchronizer ids, the reassignment id and
counter, and the submitter:

```
`-- [0] UNASSIGN Asset:Asset
    |-- contract: 00aabbcc0011...
    |-- reassignment: sync-a::... -> sync-b::...
    |-- reassignment id: reassign-0001
    |-- counter: 1
    |-- submitter: Alice
    `-- witnesses: Alice
```

An `ASSIGN` event also carries the reassigned contract's payload and
stakeholders, which the Ledger API nests inside the assigned event's created
event.

## open

Reopen an exported artifact, with no ledger connection:

```bash
dpm trace open trace.json
dpm trace open trace.json --visualize
```

## prepare

Prepare a command without committing it. This calls Canton's non-committing
prepare API and does not submit:

```bash
dpm trace prepare \
  --submitter http://localhost:<json-ledger-api-port> \
  --act-as '<party-id>' \
  --template '<package-id>:Counter:Counter' \
  --arg owner='<party-id>' \
  --arg count=0 \
  --export prepared.json
```

Or pass a command file:

```bash
dpm trace prepare \
  --submitter http://localhost:<json-ledger-api-port> \
  --act-as '<party-id>' \
  --commands commands.json \
  --export prepared.json
```

## submit

Submit-and-wait, printing the resulting update id — the primitive integration
tests use to create state and then trace it:

```bash
dpm trace submit \
  --submitter http://localhost:<json-ledger-api-port> \
  --act-as '<party-id>' \
  --template '#<package-name>:Mod:Template' \
  --arg owner='<party-id>' --arg count=0
```

`--print-json` gives the full response; `--allow-fail` captures a rejected
submission as JSON instead of erroring, which `dpm trace --completion-file` then
maps back to source.

## compare

Prepared against committed:

```bash
dpm trace compare \
  --prepared prepared.json \
  --update <update-id> \
  --submitter http://localhost:<json-ledger-api-port> \
  --read-as '<party-id>'
```

Prepared against a failed submission:

```bash
dpm trace compare \
  --prepared prepared.json \
  --command-id <command-id> \
  --submitter http://localhost:<json-ledger-api-port> \
  --act-as '<party-id>' \
  --log-file /tmp/canton-participant.log
```

Two committed transactions:

```bash
dpm trace compare <update-id-a> <update-id-b> \
  --submitter http://localhost:<json-ledger-api-port> \
  --read-as '<party-id>'
```

Prepared against captured completion JSON:

```bash
dpm trace compare \
  --prepared prepared.json \
  --completion-file completion.json
```

## test

See [unit-tests.md](unit-tests.md) and [integration-tests.md](integration-tests.md).

Scaffold a test layout once with `--init`:

```bash
dpm trace test . --init
#   created itests/lit.cfg.py, itests/example.test
#   created unittests/daml.yaml, unittests/daml/Example.daml
#   created .github/workflows/dpm-trace.yml   (unit + integration jobs)
```

`--no-unittests` / `--no-ci` skip those parts.

## Opt-in test suites

Most of the repo's own suite runs offline. These need a real toolchain:

```bash
# inspect-backed source diagnostics
DPM_TRACE_RUN_DAMLC_INSPECT=1 DPM_TRACE_DAMLC=daml \
  lit tests/completion-source-inspect.test

# the Daml Script runner end to end
DPM_TRACE_RUN_DAML_TEST=1 DPM_TRACE_DAML=daml \
  lit tests/daml-script-test.test

# a local Canton node
DPM_TRACE_RUN_REAL_CANTON=1 \
DPM_TRACE_DAML=daml \
DPM_TRACE_DAMLC=daml \
DPM_TRACE_CANTON_JAR=<path-to-canton.jar> \
DPM_TRACE_DAML_HELPER=<path-to-daml-helper> \
  lit tests/real-canton-failed-completion.test
```
