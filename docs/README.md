# dpm trace docs

User-facing documentation for `dpm trace`.

## Index

- [`commands.md`](commands.md) - full command and flag reference.
- [`unit-tests.md`](unit-tests.md) - running Daml Script tests as a source-mapped CI gate.
- [`integration-tests.md`](integration-tests.md) - running `dpm trace test --integration` against a managed local Canton.
- [`REAL-UPDATE-SMOKE.md`](REAL-UPDATE-SMOKE.md) - redacted real-Canton smoke-test checklist with placeholders.
- [`TECHNICAL-NOTES.md`](TECHNICAL-NOTES.md) - implementation notes for the trace/test internals.

## Path Hygiene

Docs must not contain local machine paths, usernames, hostnames, or personal temp
paths. Use placeholders such as `<path-to-daml-project>`,
`<path-to-canton.jar>`, `<path-to-daml-helper>`, `<package-dir>`, and
`<party-id>`.

If a local-only note is useful to future maintainers, commit a redacted version
here or link to a team-owned external location from this index.
