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

This needs [DPM](https://docs.digitalasset.com/build/3.4/dpm/dpm.html) with a
Daml SDK already installed — `install-plugin` writes into that SDK's manifest.
If you have none, `dpm install 3.4.11` first, or skip this step and use the
binary directly.

## Usage

An update id identifies a committed transaction on a participant. Canton returns
one from each successful submission, in the format
`1220e77482b473bfff30d376bd853f0a71df7ab6d41cc3f060dc5456603493acd06c`.

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

Flags and usage in full: **[docs/commands.md](docs/commands.md)**.

## Examples

[`examples/`](examples) contains three Daml examples — a create, an exercise
with a child create, and an archive — each with a trace artifact captured
against a real Canton, so the output can be seen without a ledger:

```bash
./dpm-trace open examples/create.trace.json
./dpm-trace open examples/exercise-child-create.trace.json
./dpm-trace open examples/archive.trace.json
```

These need nothing but the binary — the artifacts are committed, so there is no
ledger to reach and no Daml toolchain to install.

[`examples/README.md`](examples/README.md) shows how to reproduce them against a
local Canton, and how the same commands work against a remote participant. That
does need more: a Java runtime, a Canton jar, and a Daml SDK to build the
example package's DAR.

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
