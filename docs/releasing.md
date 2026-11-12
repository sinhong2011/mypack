# Releasing Mypack

Mypack has one version line: the app and CLI. [release-please](https://github.com/googleapis/release-please) opens a release pull request, writes `CHANGELOG.md`, and creates the `vX.Y.Z` tag when that pull request is merged. [GoReleaser](https://goreleaser.com/) builds the binaries and uploads archives to that GitHub Release. release-please does not build artifacts.

There is one package, configured at the repository root in [`release-please-config.json`](../release-please-config.json). Tags look like `v0.1.0`. They do not look like `mypack-v0.1.0`, and this repo does not keep a moving `latest` git tag. GitHub marks the newest `vX.Y.Z` Release as Latest on its own.

## What a release is

The release is the three programs `make gui` and `make cli` already build:

| Binary | Package | Role |
| --- | --- | --- |
| `mypack` | `./cmd/mypack` | GUI. With a subcommand it is the CLI. Built with `-tags mygo_noinspector`. |
| `mypack-cli` | `./cmd/mypack-cli` | Headless CLI. |
| `mypack-native-host` | `./cmd/mypack-native-host` | Browser companion native messaging host. |

GoReleaser stamps the same version the Makefile stamps for a local build:

```
-s -w
-X github.com/egoist/mygo.production=1
-X mypack.dev/mypack/internal/cli.Version=<version>
-X mypack.dev/mypack/internal/desktop/app.Version=<version>
```

`<version>` is the tag without the leading `v`. `CGO_ENABLED=0` and `-trimpath` match CI and the Makefile. Windows binaries are built without `-H=windowsgui`, because `mypack` is also the CLI and needs a console.

The source defaults stay `0.1.0-dev` (`internal/cli.Version` and `internal/desktop/app.Version`). A build that does not pass those `-X` flags reports `0.1.0-dev`. `mypack version` on a release archive reports the tag version.

These are not part of the GitHub Release:

- **Discover catalog.** `catalog/discover.json` is embedded with `go:embed` as the offline fallback (`catalog.Bundled`). Installed apps also fetch that file from the repository, which is why a catalog edit does not need a release. release-please ignores a commit whose files are all under `catalog/` or `docs/`. A commit that also changes the app still counts.
- **Documentation.** The same directory rule ignores `docs/**`, including screenshots. Root `README*.md` files are not a directory, and release-please 17.6 only excludes directory prefixes, so a root README commit is ignored only when its type does not appear in the changelog (`docs:`, `chore:`, `ci:`, `test:`, `style:`, `build:`, `refactor:`). A `feat:` or `fix:` that touches only a root README still opens a release pull request; use `docs:` for those edits.
- **Generated-app runtimes.** `make runtimes` / `scripts/release-runtimes.sh` writes a signed registry under `dist/registry/`. That version is the Makefile `VERSION` (default `1.0.0`), which is also what `make app` writes into `dist/Mypack-1.0.0.dmg`. It is a different number from the git tag. Release builds do not run `release-runtimes.sh`.
- **Browser companion.** `apps/browser-companion` is built with pnpm and is not in the archives.
- **Dev signing keys.** `dist/keys/` is gitignored. The release workflows never read it. GoReleaser writes to `dist/goreleaser/` so `release --clean` cannot delete `dist/keys/` or `dist/registry/`.

`internal/companion.HostVersion` stays `0.1.0`. It is the native-messaging protocol constant, and Go `-X` cannot set a constant.

## What CI runs

CI always starts. It does not use a workflow `paths` filter, because a skipped workflow would leave the required check pending. A small job reads the diff and chooses the rest. The required branch-protection check is still named **`test (macos-latest)`**. That name is now the aggregate gate, not the macOS runner. The macOS, Ubuntu, and Windows test machines report as `app (macos-latest)`, `app (ubuntu-latest)`, and `app (windows-latest)`.

No protection change is required to merge this. Keep requiring `test (macos-latest)`. Do not switch the requirement to `app (macos-latest)`: that job is skipped on purpose for docs-only pull requests, and a skipped required check never completes.

If the required name is renamed later, do it in this order: add a second always-on job with the new name and wait until `main` reports it green, then change branch protection to the new name, then delete the old job. Do not delete `test (macos-latest)` first.

The gate fails when the change detector fails, or when any selected job fails or is cancelled. A job that was not selected is skipped, and that is a pass. Unknown paths, an empty diff, a new branch with no parent, and a manual run all select the app tests.

| Change | App vet and test (3 OS) | `go test ./catalog` | Companion `pnpm test` | Relative doc links | `goreleaser check` |
| --- | --- | --- | --- | --- | --- |
| `README*.md`, `docs/**` (including screenshots), `CONTRIBUTING.md`, `ARCHITECTURE.md`, and the other root policy docs | no | no | no | yes | no |
| `catalog/discover.json` only | no | yes | no | no | no |
| `internal/i18n/**` (UI strings, not README translations) | yes | included | no | no | no |
| Other Go, `go.mod`, `Makefile`, `scripts/**`, embedded assets | yes | included | no | no | no |
| `apps/browser-companion/**` only | no | no | yes | no | no |
| `.github/workflows/**`, `.github/ci/**`, `.goreleaser.yaml`, release-please config | yes | included | no | no | yes |
| Mixed docs and catalog | no | yes | no | yes | no |
| Mixed docs and code, or a rename/delete of a Go file | yes | included | if the extension changed | if docs changed | if release config changed |
| Any other path | yes | included | no | no | no |

`catalog/discover.json` is the only catalog file that skips the app matrix. `go test ./catalog` parses the embedded file (`TestBundled`). `catalog/*.go` is app code and runs the full matrix. UI catalogs under `internal/i18n/catalogs/` are embedded into the binary and run the full matrix; they are not documentation. Companion locale JSON is part of the extension and runs Vitest, not `go test`.

Pull requests diff the base commit against the head, including forks, with `contents: read`. The workflow does not use `pull_request_target`. A push to `main` diffs `github.event.before` against the new commit. Tags do not run this workflow. GoReleaser publishes only from a `vX.Y.Z` tag push or from the release-please job after that tag exists. A manual `workflow_dispatch` of CI runs the app tests. A new push to the same branch cancels the in-progress CI run. A release publish is not cancelled by a newer run.

Classifier fixtures live in [`.github/ci/fixtures`](../.github/ci/fixtures) and include the file list from the docs-only pull request that added the localized READMEs and the Library screenshot. The change-detector job runs those fixtures, the gate fixtures, and the link-checker fixtures before it selects any other job.

## How a release is cut

1. Land conventional commits on `main`.
   - `feat:` minor (`0.1.0` → `0.2.0`)
   - `fix:` patch (`0.1.0` → `0.1.1`)
   - `feat!:` or a `BREAKING CHANGE:` footer: major (`0.1.0` → `1.0.0`), including before 1.0.0
2. `.github/workflows/release-please.yml` opens or updates one pull request titled `chore(main): release X.Y.Z`. The pull request updates `CHANGELOG.md` and `.release-please-manifest.json`. It does not upload binaries.
3. Review that pull request and merge it. Merging creates git tag `vX.Y.Z` and a GitHub Release whose body is the changelog.
4. The same workflow then runs GoReleaser, which uploads archives and `checksums.txt` onto that Release and leaves the notes in place.

`.github/workflows/release.yml` also runs on a pushed `vX.Y.Z` tag. release-please uses the workflow's `GITHUB_TOKEN`. GitHub does not start a new workflow from events that token creates, so a tag created by release-please does not start `release.yml` on its own. The publish job is chained in `release-please.yml` for that reason. Pushing a tag yourself, or re-running the Release workflow on that tag, publishes again. Re-runs replace archives that are already there (`replace_existing_artifacts`); they keep the existing release notes (`mode: keep-existing`).

Repository setting required once: **Settings → Actions → General → Allow GitHub Actions to create and approve pull requests.** Without it, release-please cannot open the release pull request.

The release pull request is opened by `GITHUB_TOKEN`, so the test workflow does not start on it. The pull request should only change `CHANGELOG.md` and `.release-please-manifest.json`. To run CI on it later, pass a fine-scoped personal access token or GitHub App token as the `token` input of `googleapis/release-please-action`. Do not commit that token. Leave the input unset until the secret exists; an empty token fails the action.

## First release

There are no tags yet. The first release pull request is the initial version, not a bump of an already-tagged release.

- `initial-version` is `0.1.0`, from the `0.1.0-dev` constants in `internal/cli/cli.go` and `internal/desktop/app/app.go`. Commits included in that pull request are the contents of 0.1.0. They do not bump it to 0.2.0.
- `bootstrap-sha` is `c1d0dd36510548f910f44224950c2c82984a1ac5`, the commit that opened the public history. The first changelog starts after that commit. After the first release pull request has been merged, release-please ignores `bootstrap-sha` and the field can be deleted.
- `.release-please-manifest.json` is `{}` until the release pull request records the version it is cutting. Do not put `0.1.0` in the manifest yourself. A version written there is treated as already released, and the next pull request bumps past it.

After this change is on `main`, the next `feat:` or `fix:` (including the commit that added this pipeline, once it is merged) makes release-please open the release pull request. Before merging that pull request, check:

- The title is `chore(main): release 0.1.0`.
- The tag it will create is `v0.1.0`.
- The diff is `CHANGELOG.md` and `.release-please-manifest.json` (the manifest becomes `{ ".": "0.1.0" }`).
- No git tag is created by hand, and no second package path is added.

Local `make app` still defaults `VERSION` to `1.0.0`, so `dist/Mypack-1.0.0.dmg` and the dev runtime registry stay on that number. That default is not the git tag. Pass `VERSION=0.1.0` when you want a local bundle to match the first release.

To cut the first release as a different version, add a commit on `main` before merging the release pull request:

```
chore: set the first release version

Release-As: 1.0.0
```

release-please updates the open pull request to that version. Do not set `release-as` in `release-please-config.json`. That setting would force every later release to the same number.

## Archives

For `darwin` and `linux` (`amd64` and `arm64`): `mypack_<version>_<os>_<arch>.tar.gz`.

For `windows` (`amd64` and `arm64`): `mypack_<version>_<os>_<arch>.zip`.

Plus one `checksums.txt` of sha256 sums. The six targets are the ones `scripts/release-runtimes.sh` builds, without `darwin-universal` (that archive is a runtime built with `lipo`, not the app).

Each archive unpacks to a single directory containing:

- `mypack` (`mypack.exe` on Windows)
- `mypack-cli` (`mypack-cli.exe`)
- `mypack-native-host` (`mypack-native-host.exe`)
- `LICENSE`, `NOTICE`, `README.md`
- `CHANGELOG.md`, once the release pull request has added it

`make release` on a Mac still builds `dist/Mypack-<version>.app` and `dist/Mypack-<version>.dmg`. The GitHub Release does not include that disk image. See below.

## Registry key

`make gui` embeds `dist/keys/registry.pub` and a `file://` registry URL when the dev key exists. Release builds do not. They keep the registry URL compiled into the source:

`https://registry.mypack.dev/runtimes/registry.json`

No production public key is embedded, so a release binary will not trust a registry until one is configured. Either:

- set `MYPACK_TRUSTED_KEY` to the base64 Ed25519 public key in the environment of the Builder, or
- later, append `-X mypack.dev/mypack/internal/runtimemanager.trustedKeysB64=<key>` to the GoReleaser ldflags, with the key stored as a GitHub Actions secret.

When rotating, keep the old key in that comma-separated list. Never commit `dist/keys/`, `.env`, or the secret value.

## Code signing, notarization, and Homebrew

These need Apple and Homebrew setup that this repository does not have. The workflows do not invent certificates, API keys, or a tap token. The tag publishes the unsigned archives above.

### Notarized macOS disk image

`scripts/package-app.sh` already signs and notarizes when it is given real credentials. `make release SIGN_IDENTITY="Developer ID Application: …" NOTARY_PROFILE=mypack` builds `dist/Mypack-<version>.dmg`. The default `SIGN_IDENTITY` is `-` (ad-hoc). An ad-hoc signature does not satisfy Gatekeeper for an app downloaded from GitHub, so the release does not upload an ad-hoc DMG.

To do this from GitHub Actions later:

1. A Developer ID Application certificate, exported as a password-protected `.p12`.
2. An App Store Connect API key (`.p8`, key id, issuer id) for `notarytool`. A `NOTARY_PROFILE` keychain entry exists only on the machine that stored it, not on a GitHub-hosted runner.
3. GitHub Actions secrets for the certificate, its password, and the API key. Do not commit them.
4. A `macos-latest` job that imports the certificate into a temporary keychain, runs `scripts/package-app.sh <version> dmg` with `SIGN_IDENTITY` set, submits the DMG with `xcrun notarytool submit --key … --key-id … --issuer … --wait`, staples it, and uploads `dist/Mypack-<version>.dmg` with `gh release upload <tag>`.

### Homebrew

GoReleaser's open-source distribution can update a formula from the archives above. It needs a tap this project does not have yet:

1. A tap repository you control.
2. A token that can write to that repository, stored as an Actions secret. The default `GITHUB_TOKEN` can only write to this repository.
3. A `brews` section in `.goreleaser.yaml` that points at the tap and installs the same `vX.Y.Z` archives. Do not give the formula its own tag or a second GitHub Release.

If the repository later enables immutable releases, assets cannot be added after the Release is published. Switch release-please to `"draft": true` with `"force-tag-creation": true`, and set GoReleaser `use_existing_draft: true`, so binaries are uploaded before the Release is published. That is only needed once immutability is turned on.
