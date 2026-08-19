# dpm trace

Read Canton transactions the way you read a stack trace.

`dpm trace` turns a participant's view of a Daml transaction into a readable
event tree, steps through it interactively, and maps failed submissions back to
the line of Daml that rejected them.

![dpm trace](docs/demo.gif)

```
Trace
`-- [0] EXERCISE Asset:Asset.Split
    |-- contract: 00bcdba8f2bfd71330d170818b8226a39621d34b50699d05f23fc25a200c22c...
    |-- actors: Alice
    |-- argument: { splitQuantity: 40 }
    |-- [1] CREATE Asset:Asset
    |   `-- payload: { issuer: Issuer, name: GOLD, owner: Alice, quantity: 60 }
    `-- [2] CREATE Asset:Asset
        `-- payload: { issuer: Issuer, name: GOLD, owner: Alice, quantity: 40 }
```

When a submission fails there is no transaction to trace, so it reads the
completion instead and points at the guard that rejected it:

```
  status:     DAML_FAILURE
  message:    ... AssertionFailed (error category 9): Insufficient balance

Source diagnostics
  daml/Asset.daml:54:20
    52 |       controller owner
    53 |       do
  > 54 |         assertMsg "Insufficient balance" (quantity >= amount)
                            ^
```

## Install

Download the archive for your platform from
[Releases](https://github.com/walnuthq/dpm-trace/releases), or build it:

```bash
go build -o dpm-trace ./cmd/trace
```

The binary is self-contained — no runtime, no dependencies. Register it as a DPM
component so it runs as `dpm trace`:

```bash
./dpm-trace install-plugin     # one-time
dpm trace --help
```

Without that it still works standalone as `dpm-trace`.

## Try it without a ledger

The [`examples/`](examples) directory ships committed trace artifacts, so you
can see real output before pointing it at anything:

```bash
dpm trace open examples/exercise-child-create.trace.json
dpm trace open examples/exercise-child-create.trace.json --visualize
```

And the failure path, mapped back to source:

```bash
dpm trace --completion-file examples/failed-withdraw.completion.json \
  --daml-yaml examples/asset/daml.yaml
```

See [`examples/README.md`](examples/README.md) for the full set, including a
prepared-vs-committed comparison.

## Commands

| | |
|---|---|
| `dpm trace <update-id>` | inspect a committed transaction |
| `dpm trace --command-id <id>` | inspect a **failed** submission via completion data |
| `dpm trace open <artifact>` | reopen an exported trace |
| `dpm trace compare` | diff two transactions, or prepared vs committed |
| `dpm trace prepare` | prepare a command without committing it |
| `dpm trace submit` | submit-and-wait, print the update id |
| `dpm trace test` | run Daml Script tests as a source-mapped CI gate |

Add `--visualize` to any trace to step through it, `--export trace.json` to save
it, and `--print-json` for machine-readable output.

Pointing at a live participant:

```bash
dpm trace <update-id> \
  --submitter http://localhost:<json-ledger-api-port> \
  --read-as '<party-id>'
```

Full flags and workflows: **[docs/commands.md](docs/commands.md)**.

## Interactive visualizer

`--visualize` opens a stepper over the transaction:

```
dpm-trace> n            # next / prev / j <n> to jump
dpm-trace> vars         # the event's fields
dpm-trace> s            # the Daml source behind this step
dpm-trace> b Transfer   # break on a template, choice or party; c to continue
```

**Search.** Large transactions are mostly noise for any given question, so
navigation can be narrowed to what you care about:

```
dpm-trace> filter party Alice     # n/p now visit only Alice's events
dpm-trace> filter GOLD            # unqualified: searches every field, payloads included
dpm-trace> matches                # list what the filter selects
dpm-trace> find Transfer          # jump to the next match, leaving the filter alone
dpm-trace> filter                 # clear
```

Fields are `template`, `choice`, `party`, `contract`, `kind`, `package`, `id`
and `payload`, all matched case-insensitively on a substring — the values you
have to hand are usually partial.

**Folding.** Deep trees collapse so the tree fits on a screen:

```
dpm-trace> tree 1                 # fold everything below depth 1
dpm-trace> collapse #5:1          # or one subtree; bare form acts on the current step
dpm-trace> expand all
```

With a filter set, matches are marked in the left gutter, so you keep the
structure and can see which branch to open:

```
         EXERCISE #5:0 Settlement:Settlement.Settle
           EXERCISE #5:1 Asset:Asset.Transfer
             ARCHIVE  #5:2 Asset:Asset
* =>         CREATE   #5:3 Asset:Asset
*              CREATE   #5:4 Receipt:Receipt
         +   EXERCISE #5:5 Fee:Fee.Charge
             ... 1 event hidden (expand #5:5)
```

`help` lists every command.

## Daml Script tests as a CI gate

`dpm trace test` wraps `daml test` on the in-memory IDE ledger — no Canton node
— and adds what it does not: failed tests resolved to source with a caret,
transaction trees in the terminal and as JSON, and a structured report to
automate on.

```bash
dpm trace test .                              # all Script tests in the package
dpm trace test . --no-trees --junit out.xml   # compact CI logs + JUnit XML
```

It exits non-zero on any failure, so a CI step is one line. For integration
tests against a **real local Canton**, `--integration` boots a node, uploads the
DAR, allocates parties and runs a `lit` suite against it.

- **[docs/unit-tests.md](docs/unit-tests.md)** — the unit-test workflow
- **[docs/integration-tests.md](docs/integration-tests.md)** — managed Canton + lit

## Notes

- Output is participant-scoped. It is not a global Canton transaction view.
- Failed submissions may have no update id; those workflows use completion data.
- Source diagnostics prefer `damlc inspect` plus local project metadata, falling
  back to local source matching.

## Development

Build, then run the suites:

```bash
go build -o /tmp/dpm-trace ./cmd/trace
DPM_TRACE_BIN=/tmp/dpm-trace lit tests
go test ./...
```

`lit` and `FileCheck` come from pip: `pip install lit filecheck`. See
[AGENTS.md](AGENTS.md) for the code layout and contribution rules, and
[docs/](docs) for the rest.
