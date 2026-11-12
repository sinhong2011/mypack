# Releasing Mypack

Mypack has one version line: the app and CLI. A release is a `vX.Y.Z` tag on `main`, as in [MyGo](https://github.com/egoist/mygo). There is no release pull request, no version file to bump, and no `CHANGELOG.md`. Pushing the tag starts [`.github/workflows/release.yml`](../.github/workflows/release.yml), which tests the tagged commit and runs [GoReleaser](https://goreleaser.com/) to build the archives and create the GitHub Release.

Tags look like `v0.1.0`, or `v0.2.0-beta.1` for a pre-release. They do not look like `mypack-v0.1.0`, and this repo does not keep a moving `latest` git tag. GitHub marks the newest stable `vX.Y.Z` Release as Latest on its own.

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

- **Discover catalog.** The app reads `catalog/discover.json` from `main`, so a catalog edit reaches users without a release.
- **Generated-app runtimes.** `make runtimes` / `scripts/release-runtimes.sh` writes a signed registry under `dist/registry/`. That version is the Makefile `VERSION` (default `1.0.0`), which is also what `make app` writes into `dist/Mypack-1.0.0.dmg`. It is a different number from the git tag. Release builds do not run `release-runtimes.sh`.
- **Browser companion.** `apps/browser-companion` is built with pnpm and is not in the archives.
- **Dev signing keys.** `dist/keys/` is gitignored. The release workflows never read it. GoReleaser writes to `dist/goreleaser/` so `release --clean` cannot delete `dist/keys/` or `dist/registry/`.

`internal/companion.HostVersion` stays `0.1.0`. It is the native-messaging protocol constant, and Go `-X` cannot set a constant.

## How a release is cut

On an up-to-date `main` with a clean tree:

```sh
scripts/release.sh 0.1.0          # or 0.2.0-beta.1 for a pre-release
```

The script checks that you are on `main`, that `main` matches GitHub's, and that the tag is new. It lists the commits since the last tag, asks once, then creates the annotated tag `v0.1.0` and pushes only that tag. It finds the GitHub remote by its URL, so it works when `origin` is another server.

The Release workflow then:

1. **verify**: checks that the tag is `vX.Y.Z` or `vX.Y.Z-pre` (`scripts/release.sh --check-tag`) and that its commit is on `main`, then runs `go vet ./...` and `go test ./...` on it. A tag pushed by hand, or before CI on `main` finished, cannot publish a broken build.
2. **publish**: runs `goreleaser release --clean`. It builds the six archives, writes `checksums.txt`, and creates the GitHub Release named `Mypack X.Y.Z`. A tag with `-` in it is marked as a pre-release (`prerelease: auto`).

On pull requests that touch the release files, the same workflow runs `goreleaser check` and the script's `--self-test`, and publishes nothing.

### Choosing the version

You choose it; nothing computes it from commit messages. Before 1.0.0, use the minor version (`0.2.0`) for new features or anything that breaks, and the patch version (`0.1.1`) for fixes only.

### Release notes

GoReleaser uses `changelog.use: github-native`: the notes are what GitHub's *Generate release notes* writes, the titles of the pull requests merged since the previous tag, plus new contributors. Pull request titles are checked as Conventional Commits (`.github/workflows/conventional-title.yml`) so the list reads well. Edit the notes on GitHub after publishing if a release needs a summary at the top.

To group the notes (Features, Fixes, Catalog), add [`.github/release.yml`](https://docs.github.com/en/repositories/releasing-projects-on-github/automatically-generated-release-notes) with label-based categories. Discover catalog pull requests then fall under their own heading, or can be excluded by label.

### Publishing again

Re-running the Release workflow on the same tag rebuilds the archives and replaces them on the existing Release (`replace_existing_artifacts`). Notes that are already there are kept (`mode: keep-existing`). Do not move a tag that has been published; cut the next patch version instead.

### Trying it locally

```sh
goreleaser release --snapshot --clean --skip=publish
```

builds every archive into `dist/goreleaser/` with a `-snapshot` version, without a tag and without touching GitHub.

### Local bundles

Local `make app` still defaults `VERSION` to `1.0.0`, so `dist/Mypack-1.0.0.dmg` and the dev runtime registry stay on that number. That default is not the git tag. Pass `VERSION=0.1.0` when you want a local bundle to match a release.

## Archives

For `darwin` and `linux` (`amd64` and `arm64`): `mypack_<version>_<os>_<arch>.tar.gz`.

For `windows` (`amd64` and `arm64`): `mypack_<version>_<os>_<arch>.zip`.

Plus one `checksums.txt` of sha256 sums. The six targets are the ones `scripts/release-runtimes.sh` builds, without `darwin-universal` (that archive is a runtime built with `lipo`, not the app).

Each archive unpacks to a single directory containing:

- `mypack` (`mypack.exe` on Windows)
- `mypack-cli` (`mypack-cli.exe`)
- `mypack-native-host` (`mypack-native-host.exe`)
- `LICENSE`, `NOTICE`, `README.md`

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

If the repository later enables immutable releases, a published Release cannot take more assets, and re-running the workflow cannot replace them. Set GoReleaser `release.draft: true` and publish the draft once the archives are on it.
