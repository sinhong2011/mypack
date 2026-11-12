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

- **Discover catalog.** Commits that only touch `catalog/` are excluded in `release-please-config.json`. Merging a catalog edit does not open a release pull request. A commit that also changes the app still counts.
- **Generated-app runtimes.** `make runtimes` / `scripts/release-runtimes.sh` writes a signed registry under `dist/registry/`. That version is the Makefile `VERSION` (default `1.0.0`), which is also what `make app` writes into `dist/Mypack-1.0.0.dmg`. It is a different number from the git tag. Release builds do not run `release-runtimes.sh`.
- **Browser companion.** `apps/browser-companion` is built with pnpm and is not in the archives.
- **Dev signing keys.** `dist/keys/` is gitignored. The release workflows never read it. GoReleaser writes to `dist/goreleaser/` so `release --clean` cannot delete `dist/keys/` or `dist/registry/`.

`internal/companion.HostVersion` stays `0.1.0`. It is the native-messaging protocol constant, and Go `-X` cannot set a constant.

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
