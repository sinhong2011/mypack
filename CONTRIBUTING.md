# Contributing to Mypack

Thanks for helping. Mypack's promise is simple: paste a URL, get a reliable
desktop app, and keep it updated, repairable and safe to roll back. Changes that
make that core flow faster, clearer or more reliable are the most welcome.

## Ways to help

- **Add a site to Discover.** Edit [`catalog/discover.json`](catalog/discover.json):
  add `name`, an `https://` `url` and a `category` (`work`, `dev`, `chat`,
  `ai`, `design` or `media`), then run `go test ./catalog`. Once merged, the
  site shows up in everyone's Discover page within a day, no release needed.
  release-please ignores commits that only touch `catalog/`.
  Entries only fill in an address; they can't carry scripts or settings.
- **Website compatibility reports.** A site that doesn't log in, loses its
  session, opens links in the wrong place or looks broken in a generated app.
  Use the *Website compatibility* issue form.
- **Platform testing.** Try builds on macOS, Windows and Linux and report what
  happens. Clear reproduction steps are worth as much as code.
- **Bug fixes and runtime work.** See "Working on the code" below.
- **Translations.** Strings live in `internal/i18n/catalogs/*.json`. English
  (`en.json`) is the source; other languages fall back to it.
- **Docs and troubleshooting.** Anything that helps the next person get unstuck.

For a new feature, open a discussion or an issue first so we can agree on the
scope before you spend time on it. Small fixes can go straight to a pull
request.

## Working on the code

Requires Go ≥ 1.27.3 (`GOTOOLCHAIN=auto` fetches it).

```sh
make test       # go test ./...
make vet        # go vet for macOS, Windows and Linux
make run-fake   # the GUI against an in-memory demo service, no builds needed
make runtimes   # cross-compile + sign dev runtimes into dist/registry
make run        # the real GUI against your dev registry
```

`make runtimes` creates a development signing key in `dist/keys/`. Never commit
it; `dist/` is ignored.

Read [ARCHITECTURE.md](ARCHITECTURE.md) before larger changes. In short:

- `internal/appservice` is the one service that the GUI, the CLI and the MCP
  server all use. Put behaviour there, not in a front end.
- Errors are `*domain.Error` with a stable `Code`. The CLI maps codes to exit
  codes, so don't rename existing ones.
- Generated apps (`runtime/`) must keep working when Mypack is uninstalled.

## Releases

App and CLI versions are cut from `main` with [release-please](docs/releasing.md).
Use [Conventional Commits](https://www.conventionalcommits.org/): `feat:` bumps
the minor version, `fix:` bumps the patch, and `feat!:` or a `BREAKING CHANGE`
footer bumps the major. Merging the release pull request creates the `vX.Y.Z`
tag. Do not push that tag yourself.

## Pull requests

- Keep each pull request to one change, with tests where it is practical.
- Start the title with a Conventional Commit type, for example `docs: update the library screenshot`. The title check accepts `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`, `chore`, `ci`, and `revert`.
- Run `make test` and `make vet` before you push.
- New user-facing text goes in `en.json`. Translations for other languages are
  welcome but not required.
- Describe what you changed and how you checked it. Screenshots help for UI
  changes.

## License of contributions

Mypack is licensed under the [Apache License 2.0](LICENSE). Unless you say
otherwise, any contribution you submit is licensed under the same terms, as
described in section 5 of the license. There is no separate contributor
agreement to sign.

## Conduct

Everyone taking part is expected to follow the
[Code of Conduct](CODE_OF_CONDUCT.md).
