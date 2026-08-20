# Technical Notes

Operational notes for `dpm trace`, without local environment details. The code
layout and contribution rules live in [AGENTS.md](../AGENTS.md).

## Participant Scope

Ledger API fetches are participant-scoped and require `--read-as` (alias
`--party`). Do not describe the resulting tree as a global Canton transaction:
it is the view visible to the supplied parties on that participant. The same
update read as a different party can legitimately contain fewer events.

The projection is labelled in every rendering — the `visibility:` line in the
tree output, and `privacy.scope` in the JSON artifact — so a saved artifact
cannot be mistaken for a global record later.

## Participant Endpoints

`--submitter`, `--participant-url` and `--ledger-url` are the same flag: the
JSON Ledger API base URL. `--submitter` is the spelling the proposal and the
original CLI use, so it is the one to prefer in documentation, even though it
names a URL rather than a submitting party.

Bearer tokens come from `--token`, `--token-file` (alias `--access-token-file`),
or the environment as `DPM_TRACE_TOKEN` / `DPM_TRACE_TOKEN_FILE`. Prefer a file
or the environment over an argument, which leaks into shell history and the
process list.

Because a committed update reaches other participants asynchronously, reading
one from a participant that did not host the submission may need `--wait
<seconds>`, which retries until the update becomes visible.

## Event Kinds

A transaction tree carries no archived event. The Ledger API reports an archive
as `consuming: true` on the exercise and only synthesizes `ArchivedEvent` in the
flat and ACS views, which `dpm trace` does not read. The event-count summary
therefore counts a consuming exercise as both an exercise and an archive; the
counts describe effects rather than partitioning the event list, and can total
more than the event count.

## Tree Rendering

The tree is drawn with box-drawing characters, falling back to ASCII on Windows
because `cmd.exe` on a legacy code page renders them as mojibake.
`DPM_TRACE_ASCII` overrides in both directions. The fallback keys off the
platform rather than the locale on purpose: a locale-sensitive default would
render differently on a developer machine and in CI, and the golden harness
asserts every byte.

## Source Diagnostics

Source mapping prefers package metadata from `damlc inspect`, DAR files, and
local `daml.yaml` source roots. Text matching is a fallback for user-authored
failure strings such as `assertMsg`, `abort`, and assertion messages.

Large source matches are capped by default. `--max-source-locations <n>` raises
the cap, and the reports say when the cap binds rather than implying the list is
exhaustive.

## Spawning Daml Tooling

When spawning `daml` or `damlc`, build the child environment with
`testrunner.ChildEnv`. It drops `DPM_RESOLUTION_FILE` so the child resolves the
target package rather than the dpm-trace component's plugin context, and forces
a UTF-8 locale when the inherited one is not, without which the tree characters
break in the child's output.
