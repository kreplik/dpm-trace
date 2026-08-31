# dpm trace

A DPM component for inspecting Canton transactions.

`dpm trace <update-id>` reads a committed update from a participant's JSON
Ledger API and renders it as an event tree: creates, exercises, archives and
reassignments, with the contract ids, parties, choice arguments, return values
and payloads that participant can see.

![dpm trace](docs/demo.gif)

## Install

Download the archive for your platform from
[Releases](https://github.com/walnuthq/dpm-trace/releases) and unpack it. The
binary is self-contained — no runtime, no dependencies:

```bash
tar xzf dpm-trace_<version>_<os>_<arch>.tar.gz
./dpm-trace --version
```

Building instead needs Go 1.25 or newer:

```bash
go build -o dpm-trace ./cmd/trace
```

**That is enough to use it.** Everything below works with `./dpm-trace`.

Optionally, register it as a DPM component so it runs as `dpm trace`:

```bash
./dpm-trace install-plugin
dpm trace --help
```

Registration requires DPM with a Daml SDK installed. If you have neither:

```bash
curl https://get.digitalasset.com/install/install.sh | sh   # installs dpm
dpm install 3.5.1                                           # installs an SDK
```

Otherwise keep using `./dpm-trace`; it is the same binary.

## Usage

An update id identifies a committed transaction on a participant, in the format
`1220e77482b473bfff30d376bd853f0a71df7ab6d41cc3f060dc5456603493acd06c`. Canton
returns one from each successful submission, so if you have no ledger yet, start
with the [examples](#examples) below — they need no update id and no
participant.

Against a local participant:

```bash
dpm trace <update-id> \
  --submitter http://localhost:<json-ledger-api-port> \
  --read-as '<party-id>'
```

Against an authorized remote participant, which additionally needs a bearer
token:

```bash
dpm trace <update-id> \
  --submitter https://participant.example.com \
  --read-as '<party-id>' \
  --token-file ./token.txt
```

Write the trace as a portable JSON artifact for downstream tools, or print it to
stdout:

```bash
dpm trace <update-id> --export trace.json
dpm trace <update-id> --print-json
```

`dpm trace open <artifact>` renders a saved artifact again, with no ledger
connection.

Not every submission becomes a transaction, and not every question is about one
transaction:

```bash
dpm trace prepare --submitter <url> --act-as '<party-id>' ...   # before committing
dpm trace --command-id <command-id>                             # a failed submission
dpm trace --completion-file completion.json                     # a captured failure
dpm trace compare a.trace.json b.trace.json                     # what differs
```

A failed submission has no update id, so `dpm trace <update-id>` cannot find
it — completion data is where its outcome lives.

Flags and usage in full: **[docs/commands.md](docs/commands.md)**, including
[what can and cannot be compared](docs/commands.md#what-cannot-be-compared).

## Interactive visualizer

![dpm trace --visualize](docs/visualizer.gif)

`--visualize` opens a session over the transaction instead of printing it:

```bash
dpm trace <update-id> --submitter <url> --read-as '<party-id>' --visualize
dpm trace open trace.json --visualize
```

Step through events, fold deep trees, filter by template, choice, party or
contract id, search inside large payloads, and list what the transaction
created and archived. The prompt names the parties you are reading as, because
the session shows one participant's projection and not a global record of the
transaction.

**[docs/visualizer.md](docs/visualizer.md)** covers the session in full.

## Examples

[`examples/`](examples) covers the four event kinds — a create, an exercise
with a child create, an archive, and a reassignment — each with a trace
artifact captured against a real Canton, so the output can be seen without a
ledger:

```bash
./dpm-trace open examples/create.trace.json
./dpm-trace open examples/exercise-child-create.trace.json
./dpm-trace open examples/archive.trace.json
./dpm-trace open examples/unassign.trace.json
./dpm-trace open examples/assign.trace.json
```

These need nothing but the binary — the artifacts are committed, so there is no
ledger to reach and no Daml toolchain to install.

[`examples/README.md`](examples/README.md) shows how to reproduce the first
three against a local Canton, and how the same commands work against a remote
participant. That does need more: a Java runtime, a Canton jar, and a Daml SDK
to build the example package's DAR. The reassignment pair needs two
synchronizers, which the shipped config does not set up.

## Notes

- Output is one participant's projection, not a global view of the transaction.
  It does not imply access to private data outside that participant's rights.

## Development

```bash
go build -o /tmp/dpm-trace ./cmd/trace
DPM_TRACE_BIN=/tmp/dpm-trace lit tests
go test ./...
```

`lit` and `FileCheck` come from pip: `pip install lit filecheck`.

[AGENTS.md](AGENTS.md) describes the code layout and contribution rules;
[docs/](docs) holds the rest of the documentation.

## License

Apache License 2.0. See [LICENSE](LICENSE).
