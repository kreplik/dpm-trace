# The interactive visualizer

`--visualize` opens a session over one transaction instead of printing it and
exiting. It is for the questions a static tree answers badly: which events
involved a party, what a forty-field contract actually contains, what the
transaction left behind.

```bash
dpm trace <update-id> --submitter <url> --read-as '<party-id>' --visualize
dpm trace open trace.json --visualize
```

Both forms are the same session; the second needs no ledger.

## What you are looking at

**The visualizer shows one participant's projection of a transaction, not a
global record of it.** A participant renders only what the parties you read as
are entitled to see. Another party, or the same party on another participant,
can legitimately see fewer events — or the same events under different ids.

The session keeps this in front of you rather than stating it once:

```
dpm-trace[Issuer]>
dpm-trace[Alice,Issuer] filter:kind create>
```

The prompt names the parties being read as, and the active filter. The header
carries the full `projection:` note, and `diff` repeats it in terms of what you
cannot see.

Concretely, one transfer read two ways:

```
dpm-trace[Alice]> => └── EXERCISE 0 Asset:Asset.Transfer
                         └── CREATE   1 Asset:Asset

dpm-trace[Bob]>   => └── CREATE   0 Asset:Asset
```

Alice sees the consuming exercise and the create under it. Bob, on the other
participant, sees only the create — the exercise on Alice's contract is not in
his projection at all. Note his create is event `0` where Alice's is `1`: each
participant numbers only the events it witnessed. An event id is meaningful
within one projection, not across them.

Absence is therefore not evidence. A contract you cannot see is not a contract
that does not exist.

## Moving around

| Command | Effect |
|---|---|
| `n` / `next`, `p` / `prev` | one step |
| `j <n>` | jump to a step number |
| `tree` | the whole transaction, cursor marked |
| `tree <depth>` | fold everything below a level |
| `collapse [id\|all]`, `expand [id\|all]` | hide or reveal one subtree |
| `q` / `quit` | leave |

Steps are numbered from 1; events carry the ids the ledger gave them. Those are
different numbers, and both are shown where it matters. Commands that take an
event accept the id, the id with a leading `#`, or the step number.

Folding is what makes a deep transaction readable:

```
=> └── EXERCISE #5:0 Settlement:Settlement.Settle
       ├── EXERCISE #5:1 Asset:Asset.Transfer
       │   ... 3 events hidden (expand #5:1)
       └── EXERCISE #5:5 Fee:Fee.Charge
           ... 1 event hidden (expand #5:5)
```

Counts are transitive — three hidden means three lines you cannot see, not
three direct children — and the line names the command that reopens it.
Collapse is keyed by event, so a subtree stays closed while you step away and
come back.

`collapse` and `expand` echo the event they resolved, because the argument is
ambiguous: `tree 2` means a depth, `collapse 2` means a step, and where event
ids are bare integers a digit is a plausible id too.

```
dpm-trace[Alice]> collapse 2
collapsed #5:1 (step 2)
```

## Finding things

`filter` narrows what `n` and `p` visit; it does not hide the tree.

```
filter party Alice        # only Alice's events
filter template Asset     # substring, case-insensitive
filter GOLD               # unqualified: every field, payloads included
matches                   # list what the filter selects
find Transfer             # jump to the next match, leaving the filter alone
filter                    # clear
```

Fields are `template`, `choice`, `party`, `contract`, `kind`, `package`, `id`
and `payload`. Matching is substring and case-insensitive, because the values
you have to hand are usually partial — the tail of a contract id copied from a
log, a party without its fingerprint.

With a filter set, `tree` marks matches in a fixed left gutter, so the
structure survives:

```
     └── EXERCISE #5:0 Settlement:Settlement.Settle
         ├── EXERCISE #5:1 Asset:Asset.Transfer
         │   ├── ARCHIVE  #5:2 Asset:Asset
* =>     │   └── CREATE   #5:3 Asset:Asset
*        │       └── CREATE   #5:4 Receipt:Receipt
```

A filter matching nothing is refused rather than applied — navigating with
every step excluded, and no way to see why, is worse than an error.

## Large values

A contract with forty fields would print seventy lines and push the event's own
identity off the screen. Values are cut instead:

```
payload:
  {
    "field01": "value-01",
    ...
  ... 55 lines hidden (`payload` to expand, `payload <text>` to search)
```

`payload` toggles the full rendering. `payload <text>` prints only the matching
lines of this event's payload, argument and result — which is what makes a
collapsed value usable: it answers "is this in there" without the other fifty
lines. Where `filter` chooses which events to visit, `payload` chooses which
lines of one event to read.

## What the transaction changed

`diff` lists the contracts created and archived:

```
state diff: 2 contracts created, 1 contract archived
  + created
    1    Asset:Asset 00ae317721d55e2c...
      { issuer: Issuer, name: GOLD, owner: Alice, quantity: 60 }
    2    Asset:Asset 00f0bcfd7bdd9440...
      { issuer: Issuer, name: GOLD, owner: Alice, quantity: 40 }
  x archived
    0    Asset:Asset 00bcdba8f2bfd713...
  visible to Issuer only; the transaction may have touched other contracts
```

Payloads are shown because they are what distinguishes two contracts of one
template — which of a `Split`'s outputs is the 40 and which the 60.

The archived side carries no payload. A transaction tree has no archived event:
the Ledger API reports an archive as `consuming: true` on the exercise, which
names the contract that ceased to exist but not its fields. So the diff can say
a contract was archived, not what it contained.

## One event in detail

`vars` prints the current event's fields — id, kind, template, contract,
choice, `consuming`, parties, arguments, results, children, and the source
location when one is known. `json` prints the event in the artifact encoding,
for piping elsewhere. Large values are bounded here too, and `payload` expands
them.

## Breakpoints

```
b Transfer      # break on a template, choice, party, or source location
b 3             # or a step number
bp              # list
clear [n]       # remove one, or all
c / continue    # run to the next
```

## Source

`s` shows the Daml behind the current step:

```
/path/to/FailureDemo.daml:14
  12 |     choice Withdraw : ContractId GuardedAccount
  13 |       with
> 14 |         amount : Int
```

This needs a `daml-debug-info/v1` file, passed with `--debug-info`, which is
what maps a template or choice to where it is defined. `--daml-yaml`,
`--source-root` and `--dar` serve a different purpose — matching failure text
such as an `assertMsg` string back to a line — and do not produce per-event
locations.

Without matching metadata the visualizer says so rather than guessing, and
every other command keeps working. Source mapping is best-effort throughout:
source-linked replay shows unsupported expressions as `(not evaluated)`.

## Everything else

`context` reports what the trace does and does not contain. `help` lists every
command in the session.
