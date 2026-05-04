# dpm trace POC

CLI-only POC for adding a DPM command:

```bash
dpm trace
dpm trace --interactive
dpm trace bundle
dpm trace replay
dpm trace simulate
```

The point is to prove a small slice of the proposal:

- register `trace` through DPM's component command model
- distinguish public Scan API data from authenticated participant Ledger API data
- normalize a Canton update into a trace artifact
- step through a committed transaction event-by-event in a terminal
- capture a replay/debug bundle with participant-visible ACS state
- prepare a real non-committing simulation through the participant
- step through replay bundles with source breakpoints and visible variables
- step into simple Daml choice-body expressions from replay context

This is not a patch to DPM core. It is a local DPM component that contributes a `trace` command.

## Quick Start

Install the package in the local virtualenv:

```bash
cd /Users/djtodorovic/projects/crypto/CANTON/dpm-trace
.venv/bin/python -m pip install -e .
```

Install the local DPM component:

```bash
./scripts/install-local-dpm-trace.sh
```

Run the command through DPM:

```bash
cd /Users/djtodorovic/projects/crypto/CANTON/dpm-trace
```

For the real participant-backed flow, configure the participant once:

```bash
cp .dpm-trace.example.json .dpm-trace.json
# edit .dpm-trace.json with the local JSON Ledger API URL, read-as party, DAR path, and matching debug-info path
```

Then the usage is:

```bash
DPM_HOME=$PWD/.dpm-home $HOME/.dpm/bin/dpm trace '<update-id>'
DPM_HOME=$PWD/.dpm-home $HOME/.dpm/bin/dpm trace --interactive '<update-id>'
```

`dpm trace` pretty-prints a Foundry-style event tree by default. When matching compiler debug-info metadata is configured or present in the bundle, the trace includes compact source anchors such as `source: Counter.daml:15 (Counter:Counter.Increment)`.
Source mapping comes from a compiler-emitted `--debug-info` JSON sidecar keyed by package id. The tool no longer guesses source locations by scanning `.daml` files.
ANSI color is enabled for TTY output unless `NO_COLOR` is set. The same color handling is used in interactive replay mode for step headers, source snippets, expression results, variables, and breakpoints.
Full Canton party ids such as `Alice::1220...` are rendered as `Alice` in the trace, with a party legend in the header. The suffix is a Canton namespace/fingerprint, not something the tool can reverse into a human name.

```bash
DPM_HOME=$PWD/.dpm-home $HOME/.dpm/bin/dpm trace '<update-id>' --color always
DPM_HOME=$PWD/.dpm-home $HOME/.dpm/bin/dpm trace '<update-id>' --color never
DPM_HOME=$PWD/.dpm-home $HOME/.dpm/bin/dpm trace '<update-id>' --debug-info ./counter.debug-info.json
DPM_HOME=$PWD/.dpm-home $HOME/.dpm/bin/dpm trace '<update-id>' --print-json
```

The compiler POC adds:

```bash
damlc build --experimental-debug-info
```

This emits a sidecar next to the DAR, for example `counter-example-1.0.0.debug-info.json`. The file contains `schema: daml-debug-info/v0`, the package id, source root, source file hashes, template/choice spans, and IDE expression spans. `dpm trace --debug-info ...` consumes that sidecar instead of guessing source locations by scanning source names.

To capture the replay/debug context around a committed update:

```bash
DPM_HOME=$PWD/.dpm-home $HOME/.dpm/bin/dpm trace bundle '<update-id>' \
  --out /tmp/counter.bundle.json

DPM_HOME=$PWD/.dpm-home $HOME/.dpm/bin/dpm trace replay /tmp/counter.bundle.json
DPM_HOME=$PWD/.dpm-home $HOME/.dpm/bin/dpm trace replay /tmp/counter.bundle.json --interactive
```

`bundle` fetches the update, captures an ACS snapshot at the pre-update offset when the participant JSON API allows it, attaches configured DAR paths, and infers a partial replay command for simple root create/exercise updates.

On the local Counter example, this currently produces:

```text
Replay bundle
  ACS:          captured at offset 33, 0 active contracts
  packages:     attached
  command:      available (inferred-from-ledger-effects, partial)
  engine hooks: missing
```

The higher-level simulation command starts from the committed update id:

```bash
DPM_HOME=$PWD/.dpm-home $HOME/.dpm/bin/dpm trace simulate '<update-id>'
DPM_HOME=$PWD/.dpm-home $HOME/.dpm/bin/dpm trace simulate '<update-id>' --override amount=1000
```

It fetches the update, captures the replay context in memory, infers a partial command for simple root create/exercise updates, optionally applies `--override key=value` changes to the reconstructed create arguments or choice argument, and sends that command to:

```text
POST /v2/interactive-submission/prepare
```

That is Canton's engine-backed, non-committing preparation flow. On the local Counter example it returns a prepared transaction, prepared transaction hash, hashing scheme, and traffic cost estimate without recording anything on the ledger.
This path requires an authorized participant Ledger JSON API endpoint. A CantonScan URL can identify the update, but Scan alone cannot run the participant prepare call.

When replay context is available, `simulate` extracts `createdEventBlob` values from the captured ACS snapshot and attaches them as `disclosedContracts`. This lets the participant interpreter resolve contracts that were active before the committed update even if they have already been consumed in the current ledger state.

`--override` is intentionally narrow in this POC. It changes command arguments, not participant database state. For create commands it updates `createArguments`; for exercise commands it updates `choiceArgument`. Nested object fields can be addressed with dot paths such as `--override choiceArgument.transfer.amount=1000`.

This is the useful boundary for the proposal: proposed-command simulation, participant-scoped historical input replay, and source/expression stepping for common replayable choice bodies are available through the participant plus local source metadata. Complete Speedy/LF interpreter stepping still needs either a local Daml-LF engine adapter or participant-side debug events.

You can also simulate an explicit command directly:

```bash
DPM_HOME=$PWD/.dpm-home $HOME/.dpm/bin/dpm trace simulate \
  --ledger-url http://localhost:6113 \
  --act-as 'Alice::1220...' \
  --template '<package-id>:Counter:Counter' \
  --arg owner='Alice::1220...' \
  --arg count=0
```

The same defaults can be supplied through environment variables:

```bash
export DPM_TRACE_LEDGER_URL=http://localhost:6113
export DPM_TRACE_READ_AS='Alice::1220...'
export DPM_TRACE_DAR=/path/to/app.dar
```

Useful commands inside the REPL:

```text
n / next     next event
p / prev     previous event
s / source   show Daml source around the current step
expr         list source expression steps for the current event
si / step-in step through source expressions inside the current event
vars         show visible step variables
b <spec>     set breakpoint by event id, template.choice, or file:line
bp           list breakpoints
c            continue to next breakpoint
tree         show transaction tree
context      show replay/debug context assessment
replay       explain local replay requirements
json         print current normalized event
q            quit
```

## How Trace Works

`dpm trace <update-id>` does not execute a Daml command. It inspects an already committed update.

The current flow is:

```text
update id or CantonScan update URL
-> load participant/Scan connection settings
-> fetch the update from an API
-> normalize the returned events
-> reconstruct the visible event tree
-> pretty-print the trace
```

There are two API modes:

- Participant Ledger API mode calls `POST /v2/updates/update-by-id` with `TRANSACTION_SHAPE_LEDGER_EFFECTS` and a `readAs` party filter.
- Scan API mode is intended for public explorer-style lookup, where the tool starts from a CantonScan-style update URL.

The important Canton-specific point is that the result is not a global transaction. It is the projection visible from the API being queried:

- for a participant Ledger API, it is the authorized participant-visible projection for the supplied party rights
- for Scan, it is the public/indexed projection exposed by the Scan service
- for multi-participant debugging, the tool needs one authorized endpoint per participant projection it wants to compare

The renderer then:

- normalizes create/exercise/archive events
- reconstructs parent/child relationships from event node ids and descendant ranges
- simplifies Ledger API value wrappers into Daml-shaped values where possible
- renders parties like `Alice::1220...` as `Alice`, while keeping the full party id in a legend
- keeps opaque identifiers such as contract ids and package ids shortened unless metadata is available

The current `--interactive` mode is event-level stepping over this same trace artifact. It can move through committed events, show arguments, payloads, witnesses, actors, and state changes, but it does not pause the original Daml execution because that execution already happened.

ACS snapshots are a separate layer. They are not needed for basic trace viewing or event-level stepping. They become relevant for replay and simulation:

- `StateService.GetActiveContracts` can provide the participant-visible active contract set at a ledger offset.
- For replay bundles, snapshots can capture visible ledger state around an update.
- For simulation, a snapshot can seed the local state against which a proposed command is evaluated.

The POC command for this is:

```bash
dpm trace bundle <update-id>
```

The bundle stores:

- normalized participant-visible transaction tree
- participant URL and read-as context
- package ids referenced by the update
- configured local DAR paths, if provided
- participant-visible ACS snapshot at the derived pre-update offset, if available
- an inferred partial replay command for simple root create/exercise updates
- privacy labels explaining that private data outside the projection is not present

Snapshots alone are not enough for true replay/debugging. A replayable context also needs packages/DARs, source metadata, command inputs, act-as/read-as context, ledger time, disclosed contracts where relevant, and Daml-LF engine/debug hooks. For arbitrary historical updates, the original command envelope is not guaranteed to be recoverable, so the tool either infers a partial replay command from visible root events or requires capture at submission time.

## Real Step-by-Step Debugging

The current `--interactive` mode is trace stepping: it walks through events that already happened.

The target debugger is stronger: re-execute a command locally from a captured participant-visible context, then pause at execution steps, source locations, or transaction nodes.

That requires a replay/debug bundle:

```text
committed update id
-> participant-visible transaction tree
-> package ids and DAR/source metadata
-> command inputs, if recoverable or captured
-> act-as/read-as party context
-> ledger time / effective time
-> visible input contracts and key state
-> ACS snapshot at the correct offset
-> Daml-LF execution/debug adapter
```

For transactions submitted through this tool, we can capture most of that at submission time:

```text
get current ledger offset
-> capture ACS snapshot at that offset
-> submit command
-> capture resulting update id and trace
-> store command envelope + pre-state + packages as a replay bundle
```

For an arbitrary historical update id, the debugger can only upgrade to true replay when enough context is recoverable:

- the relevant packages/DARs are available
- the original command arguments are visible or provided
- the input contracts are available from an ACS snapshot before the update, from history, or from a previously recorded bundle
- the participant has not pruned the required historical data
- private subtransactions outside the connected participant projection are not required

`StateService.GetActiveContracts` is the right API family for the state part of this. It can provide a participant-visible ACS snapshot at a ledger offset. That helps with replay and simulation, because the local engine needs a state view to evaluate a command against.

But ACS snapshots do not provide source-level stepping by themselves. They answer "what contracts were active and visible at this offset?" They do not answer "which Daml expression is executing now?" For that, the product needs a Daml-LF engine/debug adapter that can load the bundle, run the command, and emit step events.

So the real debugger plan is:

```text
M1: dpm trace <update-id>
    committed transaction inspection and event-level stepping

M2: dpm trace bundle <update-id>
    collect participant-visible replay context where possible

M3: dpm trace simulate <update-id>
    prepare a non-committing transaction through InteractiveSubmissionService,
    using ACS-created disclosures from replay context for historical input replay,
    with --override support for reconstructed command arguments

M4: dpm trace --interactive <bundle-or-command>
    true re-execution debugger using packages, snapshot state, command inputs, and engine hooks
```

`dpm trace simulate <update-id>` proves the participant can reconstruct a simple command from a committed update projection and run the Daml command interpreter without committing. For consumed contracts, the replay context ACS snapshot provides disclosed contracts, so the prepare call can evaluate against historical input contracts rather than only the participant's current active set.

The replay debugger now also steps into simple choice-body expressions when source and replay input state are available. For example, it can enter `create this with count = count + 1`, show `count = 0`, evaluate `count + 1`, and show the resulting created contract payload with `count = 1`.

## Source Breakpoints and Variables

Replay bundles can now be opened in a source-aware terminal debugger:

```bash
DPM_HOME=$PWD/.dpm-home $HOME/.dpm/bin/dpm trace replay /tmp/counter.bundle.json --interactive
```

The debugger builds its source index from compiler-emitted debug-info metadata:

- `--debug-info ./counter-example-1.0.0.debug-info.json`
- `DPM_TRACE_DEBUG_INFO`
- `debugInfoPaths` in `.dpm-trace.json`
- `debugInfoPaths` stored in a replay bundle

It maps transaction nodes to Daml source at the template/choice level only when the debug-info package id matches the package id in the transaction. For example, an exercise of `Counter.Increment` maps to the `choice Increment` line in `Counter.daml`.

Breakpoint examples:

```text
b Counter:Counter.Increment
b Counter.daml:15
b 0
c
```

The `vars` command shows the visible variables for the current replay step:

- event id and event kind
- template, choice, contract id
- actors, witnesses, signatories
- choice argument and result
- create payload
- input contract payload from the pre-update ACS snapshot, when present

For a replayed `Counter.Increment`, the debugger shows the historical input contract:

```text
inputContract: { owner: Alice, count: 0 }
```

## Expression Stepping

The `expr` and `si` commands add a source-expression layer on top of replay bundles:

```text
expr       list expression steps for the current event
si         step into the next expression
```

For a replayed `Counter.Increment`, `si` walks through:

```text
Expression 1  enter Increment
vars:
  count: 0
  owner: Alice
  this: { count: 0, owner: Alice }

Expression 4  evaluate count
source: Counter.daml:18
  create this with count = count + 1
expr:   count + 1
vars:
  count: 0
result: 1

Expression 5  create contract
result: { owner: Alice, count: 1 }
```

This is implemented from the replay bundle plus Daml source:

- the input contract comes from the pre-update ACS snapshot
- `this`, template fields, and choice arguments become visible variables
- simple record updates like `create this with count = count + 1` are evaluated locally
- the final result is checked against the child create event from the replayed transaction

This handles the immediate expression-level debugging need for common Daml choice bodies. A full Daml-LF interpreter adapter is still the path for arbitrary LF expression coverage, closures, pattern matches, exceptions, and every internal Speedy step.

## DPM Component Model

DPM does not discover commands from random executables on `PATH`. It builds its SDK command list from installed components.

This POC adds:

```text
component.yaml
bin/dpm-trace
scripts/install-local-dpm-trace.sh
```

`component.yaml` contributes the command:

```yaml
apiVersion: digitalasset.com/v1
kind: Component
spec:
  commands:
    - name: trace
      path: bin/dpm-trace
      desc: Inspect Canton transactions and open an interactive trace debugger
```

The install script creates a workspace-local DPM home at `.dpm-home`, links the installed official SDK components, and adds the local `dpm-trace` component to the SDK manifest. That keeps the POC isolated from your global DPM installation.

## Scan API Mode

Scan APIs are public/indexed APIs exposed by Super Validator Scan services. They are useful for a CantonScan-like flow.

```bash
DPM_HOME=$PWD/.dpm-home $HOME/.dpm/bin/dpm trace --interactive \
  --scan-url https://scan.example/api/scan \
  1220...
```

The POC calls:

```text
GET /v2/updates/{update_id}
```

Scan data is good for public update inspection. It is not the same as a bank/private participant projection.

## Ledger JSON API Mode

Ledger JSON API is participant/validator-scoped and authenticated.

```bash
DPM_HOME=$PWD/.dpm-home $HOME/.dpm/bin/dpm trace --interactive \
  --ledger-url http://localhost:7575 \
  --token-file /tmp/token.jwt \
  --read-as Alice::participant1 \
  1220...
```

If `.dpm-trace.json` or environment defaults are set, this becomes:

```bash
DPM_HOME=$PWD/.dpm-home $HOME/.dpm/bin/dpm trace 1220...
DPM_HOME=$PWD/.dpm-home $HOME/.dpm/bin/dpm trace --interactive 1220...
DPM_HOME=$PWD/.dpm-home $HOME/.dpm/bin/dpm trace bundle 1220...
```

The POC calls:

```text
POST /v2/updates/update-by-id
```

with `TRANSACTION_SHAPE_LEDGER_EFFECTS` and a party filter. This returns the participant-visible projection, not global Canton state.

## Local Two-Participant Canton 3 Devnet

For a more realistic local test than a single sandbox, use the Daml 3 Counter project with the Canton 3 config in this repo:

```bash
cd /Users/djtodorovic/projects/crypto/CANTON

java \
  -Dcounter.dar-path=/Users/djtodorovic/projects/crypto/CANTON/daml-examples/daml-3x/.daml/dist/counter-example-1.0.0.dar \
  -jar $HOME/.daml/sdk/3.4.11/canton/canton.jar \
  -c dpm-trace/examples/devnet-trace-poc.conf \
  --bootstrap daml-examples/canton-config/counter-deploy-3x.canton
```

The POC config creates:

```text
participant1 gRPC Ledger API: localhost:6111
participant1 JSON Ledger API: localhost:6113
participant2 gRPC Ledger API: localhost:6121
participant2 JSON Ledger API: localhost:6123
```

Configure `dpm trace` for `participant1`, submit a command, then inspect the resulting update:

```bash
cd /Users/djtodorovic/projects/crypto/CANTON/dpm-trace

cat .dpm-trace.json
# {
#   "ledgerUrl": "http://localhost:6113",
#   "readAs": "Alice::1220..."
# }

DPM_HOME=$PWD/.dpm-home $HOME/.dpm/bin/dpm trace '<update-id>'
DPM_HOME=$PWD/.dpm-home $HOME/.dpm/bin/dpm trace --interactive '<update-id>'
```

If you try the same update through `participant2` as Bob and Bob was not a witness, Canton returns:

```text
UPDATE_NOT_FOUND: Update not found, or not visible.
```

That is the local proof of the participant projection model. A debugger can step a committed transaction from the connected participant's visible projection. A multi-participant debugger needs one authorized endpoint per participant projection it wants to compare.

## What This Proves

M1-style interactive trace stepping is doable with existing update data:

```text
update id
-> Scan or Ledger API fetch
-> normalize transaction tree
-> step through create/exercise/archive events
```

It also shows the hard boundary for the proposal:

```text
event stepping != true execution replay
```

True replay needs a replay bundle and engine instrumentation.

## What True Step-By-Step Needs Locally

Run:

```bash
DPM_HOME=$PWD/.dpm-home $HOME/.dpm/bin/dpm trace --explain-replay
```

In short:

- verified DAR/source metadata
- visible input contracts and ACS snapshot at the right offset
- command/choice arguments and act-as/read-as context
- ledger time and disclosed-contract context
- Daml-LF engine instrumentation to pause/step execution

## Proposal Implication

The correct technical claim is:

> We can step through committed remote transactions at the trace/event level today. We can upgrade selected transactions into true replay/debug sessions when enough participant-visible context can be bundled and replayed locally.

That means the proposal should include:

- trace artifact format
- source/package registry
- replay bundle format
- engine trace/debug adapter
