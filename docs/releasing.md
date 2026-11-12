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
- **Generated-app runtimes.** These are on their own Release, `runtimes`, published by the **Runtimes** workflow ([below](#runtime-registry)). Locally, `make runtimes` / `scripts/release-runtimes.sh` writes a dev registry under `dist/registry/`, at the Makefile `VERSION` (default `1.0.0`), which is also what `make app` writes into `dist/Mypack-1.0.0.dmg`. Neither is the git tag.
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
4. **packages**: for a stable tag (no `-`), downloads those installers back from the Release, writes every package manager's manifest with [`scripts/packages.sh`](../scripts/packages.sh), and hands them to one job per package manager: **homebrew**, **scoop**, **winget**, **chocolatey**, **aur** and **linux-repo** (see [Package managers](#package-managers)). A job whose secret is not set skips with a notice, so the Release itself never fails on them.

On pull requests that touch the release files, the same workflow runs `goreleaser check` and the `--self-test` of `scripts/release.sh`, `scripts/packages.sh`, `scripts/macos-notary-credentials.sh`, and `scripts/bundle-runtime.sh`, and builds the installers without uploading them, so a broken installer build shows on the pull request.

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

Re-running the Release workflow on the same tag rebuilds the archives and installers and replaces them on the existing Release (`replace_existing_artifacts`, `--clobber`). "Re-run failed jobs" on one installer job rebuilds that platform alone, and on one package-manager job publishes to that manager alone. Notes that are already there are kept (`mode: keep-existing`). Do not move a tag that has been published; cut the next patch version instead.

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

**Publishing updates.** `mygo.json` `updates` holds the public key and `"github": "sinhong2011/mypack"`. With `MYGO_UPDATER_PRIVATE_KEY` set, `make installers` also writes, per platform, `mypack-<version>-<os>-<arch>.tar.gz` (signed), any `*.delta` from the last published versions, and `update-<os>-<arch>.json`, plus `install.sh` (macOS and Linux), `install.ps1` (Windows) and `SHA256SUMS` (the SHA-256 of every file but the manifests; check a download with `sha256sum -c --ignore-missing SHA256SUMS`). On a tag push with the key, the installers job runs `make installers UPLOAD=1`, which is `mygo build -upload`: it uploads every one of these files with the installers to the Release `v<version>` (made by the publish job before; `-upload` would create a draft if it were missing), merges `SHA256SUMS` with the copy the other platforms already uploaded, and uploads the manifests last. Two platforms that finish at once can still drop each other's `SHA256SUMS` lines, so the `checksums` job adds any asset the file is missing afterwards. `-upload` needs `gh` with `GH_TOKEN` (`github.token`, `contents: write`) and refuses to run without the key; pull requests build without it. Installed apps read the manifests from `releases/latest/download/`, so an update is live once its Release is the latest; a pre-release is offered only to users who opted in. There is no `CHANGELOG.md`, so manifests carry no notes, and the app shows the Release's notes from the API instead.

Setup, once:

1. The key pair is made with `go run github.com/egoist/mygo/cmd/mygo@v0.9.0 keygen -o <dir>`; its public half is in `mygo.json`. Keep `mygo-update.key` out of the repository, in a password manager.
2. Add the secret: `gh secret set MYGO_UPDATER_PRIVATE_KEY < <dir>/mygo-update.key`. Without it the installers still build and the job uploads them alone, with a warning: that release has no self-updates, install scripts or `SHA256SUMS`.

Losing the key strands installed apps, which then accept only updates signed with it: ship a release with a new `publicKey`, which users must install by hand. Anyone with the key can push code to every user. Never commit it, print it in a log, or pass it on the command line.

## Runtime registry

Apps made with Mypack run on a runtime. Each platform installer ships the matching archive for `DefaultRuntimeVersion` (`pkg/config/config.go`) plus a copy of the signed `registry.json`, via `scripts/bundle-runtime.sh`. On first launch Mypack copies those into the runtime cache, so a first build on that platform does not need the network.

The production registry lives on a GitHub Release with the fixed tag `runtimes`, and is fetched only when a project asks for a runtime version or platform that is not already cached (an update, or a cross-compile):

`https://github.com/sinhong2011/mypack/releases/download/runtimes/registry.json`

That Release holds `registry.json`, its signature `registry.json.sig`, and every runtime archive, side by side. Each entry's `url` is a bare file name, so it resolves next to `registry.json`. The Release is not marked latest, so it never stands in for an app release. Publish a runtime version (below) before tagging an app release that bundles it; `make installers` downloads the archives from this Release.

**Signing key.** Mypack trusts a manifest only when it is signed with a key it knows. The production public key is committed in `internal/runtimemanager/registry.pub` and built into every binary with `go:embed`, so installers made with `mygo build` (which takes no `-ldflags`) trust it too. The private key exists only in the `REGISTRY_SIGNING_KEY` Actions secret and in your password manager. Never commit it.

**Publishing a runtime version.** Bump `DefaultRuntimeVersion` in `pkg/config/config.go` if needed, then run the workflow before you tag an app release that asks for that version:

```sh
gh workflow run runtimes.yml -f version=1.0.8
```

The workflow builds every target on macOS (for the universal binary), downloads the current `registry.json`, adds the new version to it (`signregistry registry -flat -merge`, so older versions stay listed), signs it with the next sequence number, checks that the key matches `registry.pub`, and uploads the archives, then the manifest. A version that is already on the Release is refused: archives are immutable. The manifest expires 365 days after it is signed, so publish a runtime version at least once a year.

**Rotating the key.** Generate a new pair with `go run ./scripts/signregistry keygen -keys dist/keys-new`, add the new public key as a second line of `registry.pub`, ship an app release, then set `REGISTRY_SIGNING_KEY` to the new private key. Remove the old line once no supported release needs it.

A macOS `.dmg` carries `darwin-arm64` and `darwin-amd64`. Each Windows setup and Linux package carries that architecture only. `darwin-universal` and other platforms stay on the Release for cross-builds.

**Development.** `make gui` / `make run` still use the dev key in `dist/keys/` and a `file://` registry in `dist/registry/`. `MYPACK_REGISTRY`, `MYPACK_TRUSTED_KEY` and Settings › Runtime registry override the production registry for self-hosted mirrors. `MYPACK_BUNDLED_RUNTIMES` points at a local bundle directory (the same layout as `resources/runtimes/`).

## Code signing and notarization

The workflows do not invent certificates, API keys, or an issuer. Without the secrets below, the tag publishes the unsigned archives above.

### Notarized macOS disk image

On a tag push, the macOS installer job signs the app with a Developer ID and notarizes the DMG. Signing needs the certificate. Notarization uses an App Store Connect API key when one is configured, and an Apple ID only when no API key secret is set.

```sh
base64 -i certificate.p12 | gh secret set MACOS_CERTIFICATE   # "Developer ID Application" cert + private key
gh secret set MACOS_CERTIFICATE_PASSWORD                       # the .p12 password
```

The API key is the path that does not need `APPLE_ID` or `APPLE_APP_PASSWORD`. `ASC_KEY_P8` is the raw private key, not base64:

```sh
gh secret set ASC_KEY_P8 < AuthKey_XXXXXXXXXX.p8
gh secret set ASC_KEY_ID
gh secret set ASC_ISSUER_ID
```

Create a Team key as an Admin: App Store Connect › Users and Access › Integrations › App Store Connect API › Team Keys. Download the `.p8` once; Apple does not keep a copy. `ASC_KEY_ID` is that key's Key ID. `ASC_ISSUER_ID` is the Issuer ID from the API Keys page, near the top of Integrations ([Generating Tokens for API Requests](https://developer.apple.com/documentation/appstoreconnectapi/generating-tokens-for-api-requests)). It is a UUID. It is not the 10-character Developer Team ID in the signing identity, and the workflow does not invent an issuer.

The job writes that private key to a file with mode 600 and stores a profile named `notary` in the same temporary keychain as the certificate:

```sh
xcrun notarytool store-credentials notary --keychain <keychain> \
  --key <file> --key-id "$ASC_KEY_ID" --issuer "$ASC_ISSUER_ID"
```

The key file is removed on success and on failure. `notarytool` validates the credentials as it stores them (its default `--validate`).

An Individual key comes from a user profile and has no Issuer ID. The same token documentation says Individual keys do not use the issuer (`iss`) claim. `notarytool` requires `--issuer` for a Team key and returns 401 Unauthorized if `--issuer` is set on an Individual key, so leave `ASC_ISSUER_ID` unset for an Individual key. The workflow then passes `--key` and `--key-id` only. [Creating API Keys for App Store Connect API](https://developer.apple.com/documentation/appstoreconnectapi/creating-api-keys-for-app-store-connect-api) says Individual keys are not able to use notarytool, so use a Team key to notarize. The workflow does not invent an issuer to try to make an Individual key pass.

Apple ID credentials are the fallback when `ASC_KEY_P8`, `ASC_KEY_ID`, and `ASC_ISSUER_ID` are all unset and these three are all set:

```sh
gh secret set APPLE_ID
gh secret set APPLE_TEAM_ID          # the (TEAMID) in the identity; not the API issuer
gh secret set APPLE_APP_PASSWORD     # app-specific password, account.apple.com
```

[`scripts/macos-notary-credentials.sh`](../scripts/macos-notary-credentials.sh) chooses the credentials. A partial set fails the job and names the missing secret: a Team key that sets `ASC_ISSUER_ID` but omits `ASC_KEY_P8` or `ASC_KEY_ID`, an API key missing `ASC_KEY_P8` or `ASC_KEY_ID`, or an Apple ID set without `APPLE_TEAM_ID` or `APPLE_APP_PASSWORD` when no API key is configured. A partial API key does not fall back to Apple ID. Secret values are not printed.

With `MACOS_CERTIFICATE` unset the step warns and the disk image is signed ad hoc. With the certificate set and `MACOS_CERTIFICATE_PASSWORD` missing, the step fails. The step **Import the certificate and the notary credentials** reads the Developer ID Application identity from the certificate and adds `macos.signingIdentity` and `macos.notarize` (`keychainProfile` `notary`) to `mygo.json` for that build only, so local and pull request builds stay ad hoc. Update archives still upload only when `MYGO_UPDATER_PRIVATE_KEY` is set. To sign locally, add the same keys to `mygo.json` by hand (`"notarize": { "keychainProfile": "notary" }`, after `xcrun notarytool store-credentials notary`) and do not commit them. The Windows setup can be signed the same way with `windows.certificate` or `windows.signCommand`.

If the repository later enables immutable releases, a published Release cannot take more assets, and re-running the workflow cannot replace them. Set GoReleaser `release.draft: true` and publish the draft once the archives are on it.

## Package managers

Stable releases (tags without `-`) are published to these, from the installers on the Release. Pre-releases go to GitHub only.

| Manager | Users run | Where it lives | Job | Secret |
| --- | --- | --- | --- | --- |
| Homebrew | `brew install --cask sinhong2011/tap/mypack` | `Casks/mypack.rb` in [sinhong2011/homebrew-tap](https://github.com/sinhong2011/homebrew-tap) | homebrew | `PACKAGING_TOKEN` |
| Scoop | `scoop bucket add mypack https://github.com/sinhong2011/scoop-bucket`, `scoop install mypack/mypack` | `bucket/mypack.json` in sinhong2011/scoop-bucket | scoop | `PACKAGING_TOKEN` |
| winget | `winget install Mypack.Mypack` | `manifests/m/Mypack/Mypack/` in [microsoft/winget-pkgs](https://github.com/microsoft/winget-pkgs), by pull request | winget | `PACKAGING_TOKEN` |
| Chocolatey | `choco install mypack` | [community.chocolatey.org/packages/mypack](https://community.chocolatey.org/packages/mypack) | chocolatey | `CHOCOLATEY_API_KEY` |
| AUR | `yay -S mypack-bin` | [aur.archlinux.org/packages/mypack-bin](https://aur.archlinux.org/packages/mypack-bin) | aur | `AUR_SSH_PRIVATE_KEY` |
| APT | `apt install mypack` (README › Install adds the source) | `apt/` on the `gh-pages` branch, served at https://sinhong2011.github.io/mypack/ | linux-repo | `LINUX_REPO_GPG_KEY` |
| DNF, YUM, Zypper | `dnf install mypack` | `rpm/` on the same branch | linux-repo | `LINUX_REPO_GPG_KEY` |

What each installs:

- **Homebrew**: the universal `.dmg`, `Mypack.app` in `/Applications`, and the `mypack` command (a link to `Mypack.app/Contents/MacOS/Mypack`). `brew uninstall --zap` also removes the app's data.
- **Scoop, winget, Chocolatey**: the per-user setup program (`/S`). Scoop installs it into its own app folder and adds the `mypack` shim; winget and Chocolatey into `%LOCALAPPDATA%\Programs\Mypack`. Chocolatey has x64 only (it runs emulated on arm64); Scoop and winget have both.
- **APT, DNF, AUR**: the `.deb` / `.rpm` packages, in `/opt/mypack` with `/usr/bin/mypack` (`linux.command` in `mygo.json`), the desktop entry and the `mypack://` handler. `mypack-bin` unpacks the Debian package.

Try the manifests without publishing:

```sh
scripts/packages.sh fake-assets 0.2.0 /tmp/assets
scripts/packages.sh render 0.2.0 /tmp/assets /tmp/packages
```

### One-time setup

None of this is in the repository; each step is done once, by a maintainer.

1. **`PACKAGING_TOKEN`**: a classic personal access token with the `public_repo` scope, from an account that can push to `sinhong2011/homebrew-tap` and `sinhong2011/scoop-bucket`. Add it under *Settings › Secrets and variables › Actions*. A fine-grained token does not work for winget, which forks `microsoft/winget-pkgs` and opens a pull request from the fork.
2. **Scoop bucket**: create the public repository `sinhong2011/scoop-bucket` with an empty `bucket/` folder (a `README.md` with the two `scoop` commands helps).
3. **winget**: fork `microsoft/winget-pkgs` under the token's account. The automatic update needs a version already in winget, so submit the first one by hand: render the manifests from a published Release (`gh release download v0.2.0 --dir assets --pattern '*.exe' --pattern '*.dmg' --pattern '*.deb'`, then `scripts/packages.sh render 0.2.0 assets out`), check them with `winget validate out/winget/manifests/m/Mypack/Mypack/0.2.0` on Windows, and open the pull request with those three files (or `wingetcreate submit`). Later versions are opened by the winget job with [komac](https://github.com/russellbanks/Komac) through `vedantmgoyal9/winget-releaser`. Microsoft reviews each pull request; it can take a few days.
4. **Chocolatey**: create an account on [community.chocolatey.org](https://community.chocolatey.org), copy the API key from your account page into `CHOCOLATEY_API_KEY`. Every version goes through Chocolatey's automated checks and human moderation before it is listed; the first can take a while.
5. **AUR**: create an account on [aur.archlinux.org](https://aur.archlinux.org) with a dedicated SSH key (`ssh-keygen -t ed25519 -f aur -C mypack-aur`), add the public half to the account, and put the private half in `AUR_SSH_PRIVATE_KEY`. The job's first push creates `mypack-bin`.
6. **APT and RPM repositories**: make a signing key for the repositories alone, without a passphrase, and keep a copy offline:

   ```sh
   gpg --batch --passphrase '' --quick-gen-key 'Mypack packages <packages@mypack.dev>' ed25519 sign 3y
   gpg --armor --export-secret-keys 'Mypack packages' > mypack-packages.key   # into LINUX_REPO_GPG_KEY, then delete
   ```

   After the first release has run the linux-repo job, turn on GitHub Pages: *Settings › Pages › Deploy from a branch*, `gh-pages`, `/ (root)`. The job rewrites `gh-pages` as a single commit each release and keeps the newest five versions (`KEEP` in `scripts/linux-repo.sh`), so the branch does not grow with every release. Before the key expires, extend it (`gpg --quick-set-expire`), update the secret, and run the job again; users' copies of `mypack.gpg` need the new expiry, so mention it in the release notes. Rotating to a new key means every user re-downloads `mypack.gpg`.

### Publishing to one manager again

Re-run that job from the Release's workflow run. Each job only adds the manifest of the tag it ran for, so re-running it is safe. To publish a release that ran before a secret was set, use *Re-run all jobs*: the archives and installers are replaced with identical ones, and every manager is published.
