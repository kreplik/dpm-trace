# dpm trace docs

User-facing documentation for `dpm trace`.

## Index

- [`integration-tests.md`](integration-tests.md) — running `dpm trace test --integration` against a managed local Canton (prerequisites, the `--init` → `--integration` loop, lit substitutions).

## Local-only notes (intentionally not committed)

Two working notes are gitignored and live only on the author's machine:

- `docs/REAL-UPDATE-SMOKE.md`
- `docs/TECHNICAL-NOTES.md`

They are excluded because they contain local machine paths / environment
details that cannot be committed under this repo's path-hygiene rule (see
`tests/forbidden-markers.txt` and `AGENTS.md`). The gap is recorded here so
the knowledge is not silently lost: anyone who needs that content should
either (a) commit a redacted version with placeholders such as
`<path-to-canton.jar>` / `<party-id>`, or (b) move it into an external store
the team owns and link it from this file.

If you add a committed doc, add it to the index above.
