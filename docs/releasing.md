# Releasing Mypack

Mypack has one version line: the app and CLI. A release is a `vX.Y.Z` tag on `main`, as in [MyGo](https://github.com/egoist/mygo). There is no release pull request, no version file to bump, and no `CHANGELOG.md`. Pushing the tag starts [`.github/workflows/release.yml`](../.github/workflows/release.yml), which tests the tagged commit and runs [GoReleaser](https://goreleaser.com/) to build the archives and create the GitHub Release.

Tags look like `v0.1.0`, or `v0.2.0-beta.1` for a pre-release. They do not look like `mypack-v0.1.0`, and this repo does not keep a moving `latest` git tag. GitHub marks the newest stable `vX.Y.Z` Release as Latest on its own.

## What a release is

A release has archives and installers. The archives hold the three programs `make gui` and `make cli` already build:

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

The script checks that you are on `main`, that `main` matches GitHub's, and that the tag is new. It opens your git editor to write the release notes (the commits since the last tag are listed there as `;` comments to work from), shows the notes, asks once, then creates the annotated tag `v0.1.0` with the notes as its message and pushes only that tag. `--notes notes.md` takes the notes from a file instead. It finds the GitHub remote by its URL, so it works when `origin` is another server.

The Release workflow then:

1. **verify**: checks that the tag is `vX.Y.Z` or `vX.Y.Z-pre` (`scripts/release.sh --check-tag`) and that its commit is on `main`, then runs `go vet ./...` and `go test ./...` on it. A tag pushed by hand, or before CI on `main` finished, cannot publish a broken build.
2. **publish**: runs `goreleaser release --clean`. It builds the six archives, writes `checksums.txt`, and creates the GitHub Release named `Mypack X.Y.Z`. A tag with `-` in it is marked as a pre-release (`prerelease: auto`).
3. **installers**: three jobs at once, after publish, run `make installers` (see [Installers](#installers)) and upload the results to that Release with `gh release upload --clobber`: macOS on a macOS runner, Windows and Linux on Ubuntu.

On pull requests that touch the release files, the same workflow runs `goreleaser check` and the script's `--self-test`, and builds the installers without uploading them, so a broken installer build shows on the pull request.

### Choosing the version

You choose it; nothing computes it from commit messages. Before 1.0.0, use the minor version (`0.2.0`) for new features or anything that breaks, and the patch version (`0.1.1`) for fixes only.

### Release notes

A Release page has three parts, top to bottom:

1. **Your notes**, from the annotated tag's message (GoReleaser `release.header: "{{ .TagBody }}"`). Write them as MyGo and Pake do: sections such as `## Highlights`, `## Breaking changes` and `## Fixes`, one bullet per change a user would notice, leading with a bold phrase. Lines starting with `#` stay (they are headings); lines starting with `;` are dropped. Mention contributors as `@name` and GitHub adds their avatars under the notes.
2. **The generated list** (`changelog.use: github-native`): GitHub's *Generate release notes*, the titles of the pull requests merged since the previous tag, new contributors, and the Full Changelog link. Pull request titles are checked as Conventional Commits (`.github/workflows/conventional-title.yml`) so the list reads well.
3. **A footer** saying the binaries are unsigned, with a link to this page.

Leave the notes empty to publish the generated list alone. To change notes after publishing, edit the Release on GitHub; the tag keeps what you wrote first. Do not re-tag to fix notes.

To group the generated list (Features, Fixes, Catalog), add [`.github/release.yml`](https://docs.github.com/en/repositories/releasing-projects-on-github/automatically-generated-release-notes) with label-based categories. Discover catalog pull requests then fall under their own heading, or can be excluded by label.

The publish job fetches tags again after checkout, because `actions/checkout` can leave the tag without its message. Without that step the notes would be empty.

### Publishing again

Re-running the Release workflow on the same tag rebuilds the archives and installers and replaces them on the existing Release (`replace_existing_artifacts`, `--clobber`). "Re-run failed jobs" on one installer job rebuilds that platform alone. Notes that are already there are kept (`mode: keep-existing`). Do not move a tag that has been published; cut the next patch version instead.

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

## Installers

[`mygo build`](https://github.com/egoist/mygo/blob/main/docs/distribution.md), configured by [`mygo.json`](../mygo.json), makes them from `./cmd/mypack`:

| Platform | Files | Built on |
| --- | --- | --- |
| macOS | `mypack-<version>.dmg`, one universal (Apple silicon and Intel) app | macOS |
| Windows | `mypack-setup-<version>-amd64.exe`, `…-arm64.exe`: installs for the current user, no administrator needed | Ubuntu, with NSIS |
| Linux | `mypack_<version>_amd64.deb`, `…_arm64.deb`, `mypack-<version>-1.x86_64.rpm`, `…aarch64.rpm` (Debian and RPM write a pre-release as `0.1.0~beta.1`) | Ubuntu |

```sh
make installers                                    # this machine's platform, version 0.1.0-dev
make installers PLATFORM=darwin/universal VERSION=v0.2.0
make installers PLATFORM=windows/amd64,linux/amd64 # the setup program needs makensis (brew install makensis)
```

Results go to `build/<os>-<arch>/` (gitignored). The target writes the version into `mygo.json` for the build and puts the file back afterwards. It runs the `mygo` CLI of the version `go.mod` requires.

How the installed app differs from the archives:

- **Version.** `mygo build` takes no `-ldflags` of ours, so it links the `mygo.json` version and `cmd/mypack` reads it (`mygo.App.Version()`) when nothing stamped one. `mypack version` reports the release.
- **Windows.** `Mypack.exe` is a windowed program, so the CLI prints nothing in a terminal there. Use `mypack.exe` or `mypack-cli.exe` from the zip archive for the CLI on Windows.
- **No browser companion host.** `mypack-native-host` is only in the archives.
- **Signing.** The DMG is signed ad hoc, so Gatekeeper blocks it on other Macs until it is signed with a Developer ID and notarized ([below](#notarized-macos-disk-image)): right-click Mypack.app › Open, or `xattr -dr com.apple.quarantine /Applications/Mypack.app`. The Windows setup has no signature, so SmartScreen warns ("More info › Run anyway").

`make app` / `make dmg` (`scripts/package-app.sh`) still build `dist/Mypack.app` for local development. The Release does not use them.

## App updates

Mypack checks GitHub for a newer release and, where it can, installs it itself (Settings › Updates, and **Check for Updates…** in the Mypack menu; code in `internal/selfupdate` and `internal/desktop/app/updates.go`).

**Checking.** The app reads `GET /repos/sinhong2011/mypack/releases` from the GitHub API and takes the highest `vX.Y.Z` that is not a draft. Pre-releases count only when the user turns on **Include pre-releases**. It checks about 8 s after launch, then on an hourly tick once the last successful check is a day old (**Check for updates automatically**, on by default; the time is kept in `ui-state.json`). Development builds (`…-dev`) and `MYPACK_FAKE=1` never check on their own. A background check that finds a new version shows one toast per version and a count on Settings in the sidebar.

**Installing.** A copy that can replace itself (MyGo's `mygo.Updater.Enabled()`: built by `mygo build` with `updates` in `mygo.json`, and installed where the user can write, which covers the `.dmg` app in `/Applications` and the per-user Windows setup in `%LOCALAPPDATA%\Programs\Mypack`) downloads the signed update with `mygo.Updater`, shows its progress, and offers **Restart to Update**. A copy that cannot (an archive build or a Linux tarball without a manifest) gets a **Download** button to the release page instead.

**Package managers.** A copy installed by a package manager is never updated by the app. It shows "Installed with <manager>" and the command:

| Manager | Detected by | Command |
| --- | --- | --- |
| Homebrew | `<prefix>/Caskroom/mypack` (`$HOMEBREW_PREFIX`, `/opt/homebrew`, `/usr/local`) | `brew upgrade --cask mypack` |
| winget | `%LOCALAPPDATA%\Microsoft\WinGet\Packages\Mypack.Mypack_*`, or the app running from it | `winget upgrade Mypack.Mypack` |
| Chocolatey | `%ChocolateyInstall%\lib\mypack` | `choco upgrade mypack` |
| Scoop | the app in `<scoop>\apps\mypack` | `scoop update mypack` |
| APT | the app in `/opt/mypack` or `/usr`, with `/var/lib/dpkg/info/mypack.list` | `sudo apt update && sudo apt install --only-upgrade mypack` |
| AUR | `/var/lib/pacman/local/mypack[-bin\|-git]-*` | `yay -Syu <package>` |
| DNF / zypper | the app in `/opt/mypack` and `/usr/bin/dnf` or `/usr/bin/zypper` | `sudo dnf upgrade mypack` / `sudo zypper update mypack` |

winget installs the per-user setup like a manual download does, and leaves no record in a folder for that installer type. Such a copy is not detected and updates itself, and winget sees the newer version the next time it looks. If the package IDs or names change (cask `mypack`, winget `Mypack.Mypack`, Chocolatey and Scoop `mypack`), update `internal/selfupdate/source.go` to match.

**Publishing updates.** `mygo.json` `updates` holds the public key and `"github": "sinhong2011/mypack"`. With `MYGO_UPDATER_PRIVATE_KEY` set, `make installers` also writes, per platform, `mypack-<version>-<os>-<arch>.tar.gz` (signed), any `*.delta` from the last published versions, and `update-<os>-<arch>.json`. The installers job uploads them with the installers, manifests last. Installed apps read the manifests from `releases/latest/download/`, so an update is live once its Release is the latest; a pre-release is offered only to users who opted in. There is no `CHANGELOG.md`, so manifests carry no notes, and the app shows the Release's notes from the API instead.

Setup, once:

1. The key pair is made with `go run github.com/egoist/mygo/cmd/mygo@v0.9.0 keygen -o <dir>`; its public half is in `mygo.json`. Keep `mygo-update.key` out of the repository, in a password manager.
2. Add the secret: `gh secret set MYGO_UPDATER_PRIVATE_KEY < <dir>/mygo-update.key`. Without it the installers still build, the job prints a warning, and that release has no self-updates.

Losing the key strands installed apps, which then accept only updates signed with it: ship a release with a new `publicKey`, which users must install by hand. Anyone with the key can push code to every user. Never commit it, print it in a log, or pass it on the command line.

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

`mygo build` signs and notarizes the DMG when `mygo.json` says how:

```json
"macos": {
  "signingIdentity": "Developer ID Application: Name (TEAMID)",
  "notarize": { "keychainProfile": "notary" }
}
```

On GitHub Actions, the macOS installer job also needs the certificate and notary credentials in a temporary keychain. MyGo's [GitHub Actions guide](https://github.com/egoist/mygo/blob/main/docs/github-actions.md#signing-and-notarizing-macos-apps) has the step and the secrets it reads (`MACOS_CERTIFICATE`, `MACOS_CERTIFICATE_PASSWORD`, `APPLE_ID`, `APPLE_TEAM_ID`, `APPLE_APP_PASSWORD`). Add that step before **Build** in the macOS row. Do not commit the certificate or passwords. The Windows setup can be signed the same way with `windows.certificate` or `windows.signCommand`.

### Homebrew

GoReleaser's open-source distribution can update a formula from the archives above. It needs a tap this project does not have yet:

1. A tap repository you control.
2. A token that can write to that repository, stored as an Actions secret. The default `GITHUB_TOKEN` can only write to this repository.
3. A `brews` section in `.goreleaser.yaml` that points at the tap and installs the same `vX.Y.Z` archives. Do not give the formula its own tag or a second GitHub Release.

If the repository later enables immutable releases, a published Release cannot take more assets, and re-running the workflow cannot replace them. Set GoReleaser `release.draft: true` and publish the draft once the archives are on it.
