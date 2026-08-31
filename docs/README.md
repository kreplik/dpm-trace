# dpm trace docs

User-facing documentation for `dpm trace`.

## Index

- [`commands.md`](commands.md) - full command and flag reference.
- [`visualizer.md`](visualizer.md) - the interactive session: navigation, search, large values, state diff, and what a projection does not show.
- [`REAL-UPDATE-SMOKE.md`](REAL-UPDATE-SMOKE.md) - end-to-end checklist against a real local Canton participant.
- [`TECHNICAL-NOTES.md`](TECHNICAL-NOTES.md) - operational notes: participant scope, endpoints, event kinds, source diagnostics.

## Path Hygiene

Docs must not contain local machine paths, usernames, hostnames, or personal temp
paths. Use placeholders such as `<path-to-daml-project>`,
`<path-to-canton.jar>`, `<path-to-daml-helper>`, `<package-dir>`, and
`<party-id>`.

If a local-only note is useful to future maintainers, commit a redacted version
here or link to a team-owned external location from this index.
