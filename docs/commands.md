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

`--visualize` opens the interactive session over the trace instead of printing
it; see [visualizer.md](visualizer.md).

Write a portable JSON artifact for downstream tools, or print it to stdout:

```bash
dpm trace <update-id> --export trace.json
dpm trace <update-id> --print-json
```

Because a committed update reaches other participants asynchronously, `--wait
<seconds>` retries until it becomes visible.

### Event kinds

The tree renders create, exercise, archive and reassignment events. A
reassignment shows as `UNASSIGN` on the source synchronizer and `ASSIGN` on the
target, each with the source/target synchronizer ids, the reassignment id and
counter, and the submitter:

```
└── [0] UNASSIGN Asset:Asset
    ├── contract: 00aabbcc0011...
    ├── reassignment: sync-a::... -> sync-b::...
    ├── reassignment id: reassign-0001
    ├── counter: 1
    ├── submitter: Alice
    └── witnesses: Alice
```

An `ASSIGN` event also carries the reassigned contract's payload and
stakeholders, which the Ledger API nests inside the assigned event's created
event.

Both halves of one real reassignment are committed as
[`examples/unassign.trace.json`](../examples/unassign.trace.json) and
[`examples/assign.trace.json`](../examples/assign.trace.json).

## open

Reopen an exported artifact, with no ledger connection:

```bash
dpm trace open trace.json
```
