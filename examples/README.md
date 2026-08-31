# Examples

Representative Daml examples and the traces they produce: a **create**, an
**exercise with a child create**, an **archive (consuming exercise)**, and a
**reassignment** seen from both synchronizers.

Each one ships as a committed trace artifact, so you can see the output without
a Canton node, a Daml toolchain, or a network:

```bash
dpm trace open examples/create.trace.json
dpm trace open examples/exercise-child-create.trace.json
dpm trace open examples/archive.trace.json
dpm trace open examples/unassign.trace.json
dpm trace open examples/assign.trace.json
```

The Daml package behind the first three is in [`asset/`](asset), and the steps
below rebuild them from it — nothing outside this repository is required, though
reproducing them needs DPM and a Java runtime. Opening the committed artifacts
above needs neither.

## The shapes

| Artifact | Daml | What the trace shows |
| --- | --- | --- |
| `create.trace.json` | `createCmd Asset` | One create event: payload, signatory, observer, witnesses |
| `exercise-child-create.trace.json` | `Split` | A consuming exercise with two child creates nested under it, and the tuple of new contract ids as its result |
| `archive.trace.json` | `Burn` | A consuming exercise that archives without creating anything |
| `unassign.trace.json` | `Iou` | The outgoing half of a reassignment: source and target synchronizer, reassignment id, counter, submitter |
| `assign.trace.json` | `Iou` | The incoming half of the same reassignment, which additionally carries the contract's payload and stakeholders |

`Asset` is signed by `issuer` and observed by `owner`, so the projections differ
per party — which is the point of the participant-scoped labelling in the output.

## The reassignment pair

`unassign.trace.json` and `assign.trace.json` are two updates describing one
contract moving between synchronizers, so they share a contract id, a
reassignment id and a counter:

```
└── [0] UNASSIGN Iou:Iou
    ├── contract: 00e89575764e68ce627d51fa4f136402cc3320fce88707c58a20c08ddab8768...
    ├── reassignment: sync-a::1220e19b470d09961acb2d17952d2... -> sync-b::1220b13f96ccbe9db7875e1d6f72a...
    ├── reassignment id: 0012200ce3d189576eed2c5bfeb7e4ee84162e6bd9b7a406e2341d110a89230...
    ├── counter: 1
    ├── submitter: Alice
    └── witnesses: Alice
```

The synchronizer on the header line differs between the two — `sync-a` for the
unassign, `sync-b` for the assign — because each update is committed on the
synchronizer that saw that half.

These were captured against a ledger with two synchronizers, which
[`devnet-trace.conf`](devnet-trace.conf) does not set up: it runs one sequencer
and one mediator, so the steps below reproduce the first three artifacts only.

## Reproducing them against a local Canton

The repo ships a Canton config for exactly this — [`devnet-trace.conf`](devnet-trace.conf),
two participants with fixed ports — plus a bootstrap script that uploads the DAR
and allocates the parties:

Everything below needs only this repository and a Canton jar.

```bash
# Build the example package (examples/asset), from the repo root:
(cd examples/asset && dpm build)

CANTON=$(ls ~/.dpm/cache/components/canton-open-source/*/lib/canton-open-source-*.jar | sort -V | tail -1)

java -Ddpm.dar-path="$PWD/examples/asset/.daml/dist/asset-tests-1.0.0.dar" \
  -jar "$CANTON" daemon \
  -c examples/devnet-trace.conf \
  --bootstrap examples/devnet-trace.bootstrap.canton --no-tty
# => === dpm-trace examples ready: participant1 http://127.0.0.1:6113, participant2 http://127.0.0.1:6123 ===
```

Read the allocated party ids off the participant:

```bash
LEDGER=http://127.0.0.1:6113
curl -s $LEDGER/v2/parties | python3 -m json.tool | grep '"party"'

ISSUER='Issuer::1220...'     # from the output above
ALICE='Alice::1220...'
```

Beware `.dpm-trace.json`: config discovery walks up from the current directory,
so a file with placeholder values will silently supply them — a placeholder
party reaches the ledger as `non expected character 0x2e in Daml-LF Party`. Run
these commands outside a directory that has one, or pass `--config`.

**create** — the issuer is the signatory, so it submits:

```bash
CREATE_ID=$(dpm trace submit --submitter "$LEDGER" --act-as "$ISSUER" \
  --template '#asset-tests:Asset:Asset' \
  --arg issuer="$ISSUER" --arg owner="$ALICE" --arg name=GOLD --arg quantity=100)

dpm trace "$CREATE_ID" --submitter "$LEDGER" --read-as "$ISSUER" \
  --export examples/create.trace.json
```

**exercise with child create** — `Split` creates two assets from one. The owner
controls the choice; the issuer's authority comes from the parent contract:

```bash
CID=$(dpm trace "$CREATE_ID" --submitter "$LEDGER" --read-as "$ISSUER" --print-json \
  | python3 -c 'import sys,json; d=json.load(sys.stdin); print(next(e["contractId"] for e in d["eventsById"].values() if e.get("contractId")))')

SPLIT_ID=$(dpm trace submit --submitter "$LEDGER" --act-as "$ALICE" \
  --template '#asset-tests:Asset:Asset' --choice Split \
  --contract-id "$CID" --arg splitQuantity=40)

dpm trace "$SPLIT_ID" --submitter "$LEDGER" --read-as "$ISSUER" \
  --export examples/exercise-child-create.trace.json
```

**archive / consuming exercise** — `Burn` archives with no child create. Note it
burns one of the contracts `Split` produced: `Split` is consuming, so the
original `$CID` no longer exists.

```bash
BURN_CID=$(dpm trace "$SPLIT_ID" --submitter "$LEDGER" --read-as "$ISSUER" --print-json \
  | python3 -c 'import sys,json; d=json.load(sys.stdin); print(next(e["contractId"] for e in d["eventsById"].values() if e.get("kind")=="create"))')

BURN_ID=$(dpm trace submit --submitter "$LEDGER" --act-as "$ALICE" \
  --template '#asset-tests:Asset:Asset' --choice Burn --contract-id "$BURN_CID")

dpm trace "$BURN_ID" --submitter "$LEDGER" --read-as "$ISSUER" \
  --export examples/archive.trace.json
```

A consuming choice is how an archive appears in a participant projection: the
event is the `EXERCISE ... consuming: true`, not a separate archive event.

## Against an authorized remote participant

The command shape is identical; a remote endpoint additionally needs a bearer
token. Pass it from a file rather than an argument so it stays out of your shell
history and process list:

```bash
dpm trace "$UPDATE_ID" \
  --submitter https://participant.example.com \
  --read-as "$ALICE" \
  --token-file ~/.config/dpm-trace/token
```

`DPM_TRACE_TOKEN_FILE` sets the same thing in the environment, and
`.dpm-trace.json` can hold the participant URL and party so they need not be
repeated on every invocation — see
[Configuration](../docs/commands.md#configuration) for its shape.

Output is always **one participant's projection** — what that participant is
authorized to see, not a global view of the transaction. A remote participant
therefore shows you its own projection, which may legitimately contain fewer
events than your local one.
