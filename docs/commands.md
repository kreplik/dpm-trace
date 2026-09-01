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

## prepare

Build a command and have the participant compute its transaction *without*
committing it — the interactive-submission prepare step:

```bash
dpm trace prepare --submitter <url> --act-as '<party-id>' \
  --template '#pkg:Asset:Asset' --choice Transfer \
  --contract-id '<contract-id>' --arg newOwner='<party-id>' \
  --export prepared.json
```

Commands can also come from a file (`--commands commands.json`), a JSON string
(`--command-json`), or `--arg key=value` pairs.

The result is labelled for what it is:

```
Prepared command
  committed:    no
  command id:   dpm-trace-prepare-04427c1b4565
  prepared hash:FtItr2nW1XjyV+FNOyU16Wib3D1ccZrWewE42KtyBwE=
  hashing:      HASHING_SCHEME_VERSION_V2
```

**Nothing has happened on the ledger.** A prepared transaction is what the
participant *would* commit if the submission were signed and sent. No contract
exists, no party has been notified, and the prepared hash is not an update id.
`dpm trace open prepared.json` reopens it later.

## submit

Submit a command and print the update id it produced, so there is something to
trace:

```bash
UPDATE_ID=$(dpm trace submit --submitter <url> --act-as '<party-id>' \
  --template '#pkg:Asset:Asset' --arg owner='<party-id>' --arg quantity=100)

dpm trace "$UPDATE_ID" --submitter <url> --read-as '<party-id>'
```

It takes the same command flags as `prepare` — `--template`, `--choice`,
`--contract-id`, `--arg`, `--args-json`, `--args-file`, `--commands`,
`--command-json` — and differs only in committing rather than stopping at the
prepared transaction. A submission carries one or more commands and produces
one transaction, so one update id comes back however many commands went in.

When a submission is rejected there is no update id, and `submit` prints the
rejection instead: the status, the error and the choice it failed in. `-v`
renders the whole completion rather than the summary, and `--allow-fail` exits
0 so a script expecting a failure does not abort. Both apply only to that
rejection output — a submission that commits prints its update id and nothing
else. `--export` writes the raw response, the submission's own JSON rather than
a trace artifact, which is how a failure gets captured for `--completion-file`
later.

`prepare` and `submit` take the same command flags but not the same options.
The flags that render a rejection — `-v`/`--verbose` (also spelled `--full`),
`--allow-fail`, `--log-file`, `--color` and the source diagnostics — belong to
`submit`, since `prepare` never produces a completion. Each command rejects
what the other owns rather than accepting and ignoring it.

Both take `--synchronizer-id` to pick the synchronizer to submit or prepare
against. Given one the participant cannot reach, it says so:

```
✗ submission failed  INVALID_PRESCRIBED_SYNCHRONIZER_ID
```

## --command-id and --completion-file

A submission that fails never becomes a transaction, so it has no update id and
`dpm trace <update-id>` cannot find it. Its outcome lives in completion data
instead:

```bash
dpm trace --command-id <command-id> --submitter <url> --act-as '<party-id>'
dpm trace --completion-file completion.json
```

The first searches the participant's completion stream; widen it with
`--begin-exclusive` and `--completion-limit` if the submission is older than
the default window. The second reads a captured file, which needs no ledger
connection and is how a failure gets attached to a bug report.

Either way the view states the absence rather than inventing an id:

```
  update id:  -
  trace:      no committed transaction tree is available for this completion
```

**A command rejected during interpretation never reaches the ledger**, so it
produces no completion at all and `--command-id` will not find it. That is not
a bug in the lookup: there is nothing to find. Capture the submission response
and use `--completion-file`.

`--log-file` correlates a completion with operator logs, matching on command
id, submission id, trace id and correlation id. It is optional and local; the
tool never needs log access to explain a failure.

## compare

Five shapes, all of which answer "why do these two differ":

```bash
# two committed transactions
dpm trace compare a.trace.json b.trace.json
dpm trace compare <update-id-a> <update-id-b> --submitter <url> --read-as '<party>'

# a prepared command against what actually committed
dpm trace compare --prepared prepared.json --update trace.json

# a prepared command against a failure
dpm trace compare --prepared prepared.json --command-id <id> --submitter <url> --act-as '<party>'
dpm trace compare --prepared prepared.json --completion-file completion.json
```

`-v` prints the verbose form; the default is compact. `--print-json` emits
the comparison for scripting.

### What can be compared

**Between two committed transactions:** the event tree, contracts created and
archived, parties, choice arguments, payloads and return values. Differences
are reported per event:

```
  Events
    ~ exercise Asset:Asset.Transfer    newOwner: Bob::1220bc0a... → Mallory::1220ffff...
```

**Between a prepared command and a committed update:** the command id, the
commands themselves, and whether what committed matches what was prepared.

**Between a prepared command and a completion:** the command id (reported as
`match` when they are the same submission), completion status, error details,
submission id, offset, trace context and synchronizer time.

### What cannot be compared

**A failed submission has no transaction tree.** The Ledger API reports a
rejection, not an update, so there are no events, no contracts and no payloads
to diff. A prepared-versus-completion comparison tells you the submission
failed and why; it cannot tell you which event failed, because no events were
recorded.

**Archived contracts carry no payload.** A transaction tree reports an archive
as `consuming: true` on the exercise, which names the contract that ceased to
exist but not its fields. A diff can say a contract was archived, not what it
contained — except for a contract this same transaction created, whose fields
are already in the trace and which the state diff marks `~ transient`.

**Two participants' projections are not directly comparable.** Each participant
sees only what its parties are entitled to, and numbers only the events it
witnessed, so the same transaction read on two participants legitimately
differs in both event count and event ids. A diff across projections reports
those as differences, which is accurate about the artifacts and misleading
about the transaction. Compare like with like: same participant, same
`--read-as`.

**Absence is not evidence.** A contract or event missing from one side may be
outside that projection rather than absent from the ledger.

**Prepared hashes are not comparable across hashing schemes.** The prepared
hash is only meaningful alongside its `HASHING_SCHEME_VERSION`.
