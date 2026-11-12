<!--
Title: a Conventional Commit, written for the release notes, such as
"fix: keep redirects of a click inside the app on Windows". CI checks it.

For agents: read CONTRIBUTING.md and ARCHITECTURE.md first, fill in every
section, and keep each one short. Never remove the AI disclosure: name your
tool and your exact model.
-->

## What this changes

<!-- What changes and why, in one or two sentences. Link the issue: Fixes #123. -->

## How I checked it

<!-- Tests added or run, and screenshots for UI changes. -->

- [ ] `make test` passes
- [ ] `make vet` passes
- [ ] New user-facing text is in `internal/i18n/catalogs/en.json`

Tried on:

- [ ] macOS
- [ ] Windows
- [ ] Linux
- [ ] Not needed (docs, CI or catalog only)

## AI disclosure

<!-- Required. Write "None" when no AI was used. -->

- Tool and model: <!-- exact name and version, such as Claude Code with Claude Opus 5.5 (claude-opus-5-5) -->
- Used for: <!-- such as the code, the tests, the review, this description -->
- [ ] A human read every line of this change and can explain it.
