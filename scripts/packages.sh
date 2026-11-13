#!/usr/bin/env bash
# Writes the package-manager manifests for one MyPack release from the
# installers on its GitHub Release: a Homebrew cask, a Scoop manifest,
# winget manifests, a Chocolatey package and an AUR PKGBUILD. The Release
# workflow runs it and publishes each one whose secret is set. See
# docs/releasing.md, "Package managers".
#
#   scripts/packages.sh render 0.2.0 ASSETS OUT   # ASSETS holds the installers
#   scripts/packages.sh fake-assets 0.2.0 ASSETS  # stand-ins, to try render
#   TOKEN=… scripts/packages.sh push OWNER/REPO DIR  # commit DIR into a repo
#   scripts/packages.sh --self-test
set -euo pipefail

repo=sinhong2011/mypack
homepage="https://github.com/$repo"
desc="Turn any website or local HTML project into a native desktop app"
winget_id=MyPack.MyPack

sha256() {
  if command -v sha256sum >/dev/null; then sha256sum "$1" | cut -d' ' -f1
  else shasum -a 256 "$1" | cut -d' ' -f1; fi
}

# The installer names mygo build gives version $1 (see docs/releasing.md,
# "Installers"). Debian and RPM write a pre-release's "-" as "~".
assets() {
  local v=$1 p=${1/-/\~}
  dmg="mypack-$v.dmg"
  exe_amd64="mypack-setup-$v-amd64.exe"
  exe_arm64="mypack-setup-$v-arm64.exe"
  deb_amd64="mypack_${p}_amd64.deb"
  deb_arm64="mypack_${p}_arm64.deb"
  rpm_amd64="mypack-$p-1.x86_64.rpm"
  rpm_arm64="mypack-$p-1.aarch64.rpm"
}

need() {
  local f
  for f in "$@"; do
    [[ -f "$dir/$f" ]] || { echo "packages.sh: $dir/$f is missing" >&2; exit 1; }
  done
}

homebrew() {
  mkdir -p "$out/homebrew/Casks"
  cat >"$out/homebrew/Casks/mypack.rb" <<EOF
cask "mypack" do
  version "$version"
  sha256 "$(sha256 "$dir/$dmg")"

  url "$homepage/releases/download/v#{version}/mypack-#{version}.dmg"
  name "MyPack"
  desc "$desc"
  homepage "$homepage"

  livecheck do
    url :url
    strategy :github_latest
  end

  depends_on macos: :monterey

  app "MyPack.app"
  binary "#{appdir}/MyPack.app/Contents/MacOS/MyPack", target: "mypack"

  zap trash: [
    "~/Library/Application Support/mypack",
    "~/Library/Application Support/mypack-apps",
    "~/Library/Caches/dev.mypack.builder",
    "~/Library/Preferences/dev.mypack.builder.plist",
    "~/Library/Saved Application State/dev.mypack.builder.savedState",
  ]
end
EOF
}

scoop() {
  mkdir -p "$out/scoop/bucket"
  cat >"$out/scoop/bucket/mypack.json" <<EOF
{
  "version": "$version",
  "description": "$desc",
  "homepage": "$homepage",
  "license": "Apache-2.0",
  "architecture": {
    "64bit": {
      "url": "$base/$exe_amd64#/setup.exe",
      "hash": "$(sha256 "$dir/$exe_amd64")"
    },
    "arm64": {
      "url": "$base/$exe_arm64#/setup.exe",
      "hash": "$(sha256 "$dir/$exe_arm64")"
    }
  },
  "installer": {
    "script": [
      "Start-Process -Wait \"\$dir\\\\setup.exe\" -ArgumentList '/S', \"/D=\$dir\"",
      "Remove-Item \"\$dir\\\\setup.exe\""
    ]
  },
  "uninstaller": {
    "script": "if (Test-Path \"\$dir\\\\Uninstall.exe\") { Start-Process -Wait \"\$dir\\\\Uninstall.exe\" -ArgumentList '/S', \"_?=\$dir\" }"
  },
  "bin": [["MyPack.exe", "mypack"]],
  "shortcuts": [["MyPack.exe", "MyPack"]],
  "checkver": "github",
  "autoupdate": {
    "architecture": {
      "64bit": { "url": "$homepage/releases/download/v\$version/mypack-setup-\$version-amd64.exe#/setup.exe" },
      "arm64": { "url": "$homepage/releases/download/v\$version/mypack-setup-\$version-arm64.exe#/setup.exe" }
    }
  }
}
EOF
}

# winget: the three manifests of one version, for the first submission by
# hand; later versions are updated by the Release workflow (komac).
winget() {
  local w="$out/winget/manifests/m/MyPack/MyPack/$version"
  mkdir -p "$w"
  cat >"$w/$winget_id.yaml" <<EOF
PackageIdentifier: $winget_id
PackageVersion: $version
DefaultLocale: en-US
ManifestType: version
ManifestVersion: 1.6.0
EOF
  cat >"$w/$winget_id.installer.yaml" <<EOF
PackageIdentifier: $winget_id
PackageVersion: $version
InstallerType: nullsoft
Scope: user
InstallModes:
  - interactive
  - silent
UpgradeBehavior: install
Protocols:
  - mypack
ProductCode: dev.mypack.builder
Installers:
  - Architecture: x64
    InstallerUrl: $base/$exe_amd64
    InstallerSha256: $(sha256 "$dir/$exe_amd64" | tr a-f A-F)
  - Architecture: arm64
    InstallerUrl: $base/$exe_arm64
    InstallerSha256: $(sha256 "$dir/$exe_arm64" | tr a-f A-F)
ManifestType: installer
ManifestVersion: 1.6.0
EOF
  cat >"$w/$winget_id.locale.en-US.yaml" <<EOF
PackageIdentifier: $winget_id
PackageVersion: $version
PackageLocale: en-US
Publisher: MyPack
PublisherUrl: $homepage
PublisherSupportUrl: $homepage/issues
PackageName: MyPack
PackageUrl: $homepage
License: Apache-2.0
LicenseUrl: $homepage/blob/main/LICENSE
ShortDescription: $desc.
Tags:
  - desktop-app
  - webview
  - website
ReleaseNotesUrl: $homepage/releases/tag/v$version
ManifestType: defaultLocale
ManifestVersion: 1.6.0
EOF
}

chocolatey() {
  local c="$out/chocolatey"
  mkdir -p "$c/tools"
  cat >"$c/mypack.nuspec" <<EOF
<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://schemas.microsoft.com/packaging/2015/06/nuspec.xsd">
  <metadata>
    <id>mypack</id>
    <version>$version</version>
    <title>MyPack</title>
    <authors>MyPack contributors</authors>
    <owners>sinhong2011</owners>
    <projectUrl>$homepage</projectUrl>
    <licenseUrl>$homepage/blob/main/LICENSE</licenseUrl>
    <requireLicenseAcceptance>false</requireLicenseAcceptance>
    <projectSourceUrl>$homepage</projectSourceUrl>
    <bugTrackerUrl>$homepage/issues</bugTrackerUrl>
    <iconUrl>https://raw.githubusercontent.com/$repo/main/assets/AppIcon-1024.png</iconUrl>
    <releaseNotes>$homepage/releases/tag/v$version</releaseNotes>
    <tags>mypack desktop webview website app-builder</tags>
    <summary>$desc.</summary>
    <description>MyPack turns a website, or a local HTML project, into a real, standalone desktop app in a few clicks. Every app has its own name, icon and sign-in, and uses the system WebView (WebView2).</description>
  </metadata>
  <files>
    <file src="tools\**" target="tools" />
  </files>
</package>
EOF
  cat >"$c/tools/chocolateyinstall.ps1" <<EOF
\$ErrorActionPreference = 'Stop'
# The per-user setup program of the Release; /S installs it silently.
Install-ChocolateyPackage -PackageName 'mypack' -FileType 'exe' -SilentArgs '/S' \`
  -Url64bit '$base/$exe_amd64' \`
  -Checksum64 '$(sha256 "$dir/$exe_amd64")' -ChecksumType64 'sha256'
EOF
  cat >"$c/tools/chocolateyuninstall.ps1" <<'EOF'
$ErrorActionPreference = 'Stop'
$key = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\dev.mypack.builder'
if (Test-Path $key) {
  $dir = (Get-ItemProperty $key).InstallLocation
  Uninstall-ChocolateyPackage -PackageName 'mypack' -FileType 'exe' -SilentArgs '/S' -File "$dir\Uninstall.exe"
}
EOF
}

# AUR mypack-bin: the Release's Debian packages, unpacked.
aur() {
  local a="$out/aur" pkgver=${version//-/}
  mkdir -p "$a"
  cat >"$a/PKGBUILD" <<EOF
# Maintainer: MyPack contributors <$homepage>
# Written by scripts/packages.sh in $repo; edit it there.
pkgname=mypack-bin
pkgver=$pkgver
pkgrel=1
pkgdesc='$desc'
arch=('x86_64' 'aarch64')
url='$homepage'
license=('Apache-2.0')
depends=('gtk3' 'webkit2gtk-4.1')
provides=('mypack')
conflicts=('mypack')
options=('!strip' '!debug')
source_x86_64=("$base/$deb_amd64")
source_aarch64=("$base/$deb_arm64")
sha256sums_x86_64=('$(sha256 "$dir/$deb_amd64")')
sha256sums_aarch64=('$(sha256 "$dir/$deb_arm64")')

package() {
	bsdtar -xf data.tar.* -C "\$pkgdir"
}
EOF
  # What makepkg --printsrcinfo writes, without needing makepkg.
  cat >"$a/.SRCINFO" <<EOF
pkgbase = mypack-bin
	pkgdesc = $desc
	pkgver = $pkgver
	pkgrel = 1
	url = $homepage
	arch = x86_64
	arch = aarch64
	license = Apache-2.0
	depends = gtk3
	depends = webkit2gtk-4.1
	provides = mypack
	conflicts = mypack
	options = !strip
	options = !debug
	source_x86_64 = $base/$deb_amd64
	sha256sums_x86_64 = $(sha256 "$dir/$deb_amd64")
	source_aarch64 = $base/$deb_arm64
	sha256sums_aarch64 = $(sha256 "$dir/$deb_arm64")

pkgname = mypack-bin
EOF
}

render() {
  version=${1#v} dir=$2 out=$3
  base="$homepage/releases/download/v$version"
  assets "$version"
  need "$dmg" "$exe_amd64" "$exe_arm64" "$deb_amd64" "$deb_arm64"
  rm -rf "$out"
  homebrew; scoop; winget; chocolatey; aur
  (cd "$out" && find . -type f | sort)
}

fake_assets() {
  assets "${1#v}"
  mkdir -p "$2"
  local f
  for f in "$dmg" "$exe_amd64" "$exe_arm64" "$deb_amd64" "$deb_arm64" "$rpm_amd64" "$rpm_arm64"; do
    echo "stand-in for $f" >"$2/$f"
  done
}

# Copies DIR over a GitHub repository (the Homebrew tap, the Scoop bucket)
# and pushes the change, with TOKEN.
push() {
  local repo=$1 src=$2 t
  t=$(mktemp -d)
  git clone --quiet --depth 1 "https://x-access-token:${TOKEN:?}@github.com/$repo.git" "$t"
  cp -R "$src"/. "$t"/
  cd "$t"
  git config user.name "github-actions[bot]"
  git config user.email "41898282+github-actions[bot]@users.noreply.github.com"
  git add -A
  if git diff --cached --quiet; then echo "$repo: no change"; return; fi
  git commit --quiet -m "mypack ${GITHUB_REF_NAME#v}"
  git push --quiet
  echo "$repo: pushed"
}

self_test() {
  local t
  t=$(mktemp -d)
  trap 'rm -rf "$t"' RETURN
  fake_assets 1.2.3 "$t/assets"
  render 1.2.3 "$t/assets" "$t/out" >/dev/null
  if command -v ruby >/dev/null; then ruby -c "$t/out/homebrew/Casks/mypack.rb" >/dev/null; fi
  if command -v jq >/dev/null; then jq -e '.version == "1.2.3"' "$t/out/scoop/bucket/mypack.json" >/dev/null; fi
  if command -v python3 >/dev/null; then
    python3 -I -c 'import sys, xml.dom.minidom; xml.dom.minidom.parse(sys.argv[1])' "$t/out/chocolatey/mypack.nuspec"
  fi
  grep -q "pkgver=1.2.3$" "$t/out/aur/PKGBUILD"
  grep -q "InstallerUrl: https://github.com/$repo/releases/download/v1.2.3/mypack-setup-1.2.3-arm64.exe" \
    "$t/out/winget/manifests/m/MyPack/MyPack/1.2.3/$winget_id.installer.yaml"
  assets 0.2.0-beta.1
  [[ $deb_amd64 == "mypack_0.2.0~beta.1_amd64.deb" && $rpm_arm64 == "mypack-0.2.0~beta.1-1.aarch64.rpm" ]]
  echo "packages.sh: examples pass"
}

case "${1:-}" in
  render) [[ $# == 4 ]] || { sed -n '8,11p' "$0" | sed 's/^# //'; exit 2; }; render "$2" "$3" "$4" ;;
  fake-assets) [[ $# == 3 ]] || exit 2; fake_assets "$2" "$3" ;;
  push) [[ $# == 3 ]] || exit 2; push "$2" "$3" ;;
  --self-test) self_test ;;
  *) sed -n '2,11p' "$0" | sed 's/^# \{0,1\}//'; exit 2 ;;
esac
