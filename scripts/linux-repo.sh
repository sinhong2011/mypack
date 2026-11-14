#!/usr/bin/env bash
# Adds a release's Debian and RPM packages to the signed APT and RPM
# repositories that GitHub Pages serves from the gh-pages branch, at
# https://sinhong2011.github.io/mypack/. The Release workflow runs it on
# Ubuntu (apt-utils, createrepo-c, gnupg). See docs/releasing.md.
#
#   scripts/linux-repo.sh REPO PACKAGES_DIR GPG_KEY_ID
#   scripts/linux-repo.sh --self-test
#
# REPO is a working checkout of gh-pages. It must not be the in-repo site/
# directory, which is the marketing website; the workflow uses linux-repo/.
# The files written here are the root of gh-pages (apt/, rpm/, the signing
# key), which is what GitHub Pages serves. PACKAGES_DIR holds the new .deb
# and .rpm files. The newest $KEEP versions are kept (default 5).
set -euo pipefail

# site/ in this repository is the marketing website.
marketing_site_path() {
  [[ $(basename -- "$1") == site ]]
}

self_test() {
  local root workflow tmp marker remote
  root=$(cd "$(dirname "$0")/.." && pwd)
  workflow=$root/.github/workflows/release.yml
  # The v0.4.0 job failed by cloning gh-pages into site/, which is now the
  # marketing website. These are the command lines that did that.
  if grep -nE 'gh-pages "\$remote" site|mkdir site$|linux-repo\.sh site[[:space:]]|rm -rf site|cd site$' "$workflow"; then
    echo "linux-repo.sh: release.yml still checks gh-pages out into site/" >&2
    return 1
  fi
  grep -F 'git clone --depth 1 --branch gh-pages "$remote" linux-repo' "$workflow" >/dev/null || {
    echo "linux-repo.sh: release.yml does not clone gh-pages into linux-repo/" >&2
    return 1
  }
  grep -F 'linux-repo.sh linux-repo ' "$workflow" >/dev/null || {
    echo "linux-repo.sh: release.yml does not publish from linux-repo/" >&2
    return 1
  }
  grep -F 'repos/${GITHUB_REPOSITORY}/pages' "$workflow" >/dev/null || {
    echo "linux-repo.sh: release.yml does not enable GitHub Pages" >&2
    return 1
  }

  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' RETURN
  mkdir -p "$tmp/checkout/site"
  marker=$tmp/checkout/site/marker
  echo marketing >"$marker"

  # gh-pages publishes apt/ and rpm/ at the branch root, not under site/.
  git init -q -b gh-pages "$tmp/pages"
  echo packages >"$tmp/pages/index.html"
  mkdir -p "$tmp/pages/apt" "$tmp/pages/rpm"
  echo keep >"$tmp/pages/apt/keep"
  echo keep >"$tmp/pages/rpm/keep"
  git -C "$tmp/pages" add -A
  git -C "$tmp/pages" -c user.name=linux-repo-test -c user.email=linux-repo-test@example.com -c commit.gpgsign=false commit -q -m pages
  git init -q --bare "$tmp/remote.git"
  git -C "$tmp/pages" push -q "$tmp/remote.git" HEAD:gh-pages
  remote=$tmp/remote.git

  # The workflow's checkout, with a local stand-in for the GitHub remote.
  (
    cd "$tmp/checkout"
    if ! git clone --depth 1 --branch gh-pages "$remote" linux-repo >/dev/null 2>&1; then
      mkdir linux-repo
    fi
    test -f linux-repo/index.html
    test -d linux-repo/apt
    test -d linux-repo/rpm
    rm -rf linux-repo/.git
  )
  [[ $(cat "$marker") == marketing ]]

  if git -C "$tmp/checkout" clone --depth 1 --branch gh-pages "$remote" site 2>"$tmp/clone.err"; then
    echo "linux-repo.sh: cloning into site/ should fail when the marketing site is there" >&2
    return 1
  fi
  grep -q "destination path 'site' already exists" "$tmp/clone.err"
  [[ $(cat "$marker") == marketing ]]

  # No gh-pages branch yet: create the working directory and leave site/ alone.
  rm -rf "$tmp/checkout/linux-repo"
  git init -q --bare "$tmp/empty-remote.git"
  (
    cd "$tmp/checkout"
    if ! git clone --depth 1 --branch gh-pages "$tmp/empty-remote.git" linux-repo >/dev/null 2>&1; then
      mkdir linux-repo
    fi
    test -d linux-repo
    test ! -e linux-repo/index.html
  )
  [[ $(cat "$marker") == marketing ]]

  for path in "$tmp/checkout/site" "$tmp/checkout/site/" "$tmp/nested/site"; do
    mkdir -p "$path"
    if bash "$0" "$path" "$tmp/pkgs" DEADBEEF 2>"$tmp/refuse.err"; then
      echo "linux-repo.sh: should refuse $path" >&2
      return 1
    fi
    grep -q 'marketing website' "$tmp/refuse.err" || {
      echo "linux-repo.sh: unexpected refusal for $path" >&2
      cat "$tmp/refuse.err" >&2
      return 1
    }
  done
  [[ $(cat "$marker") == marketing ]]

  mkdir -p "$tmp/empty"
  if bash "$0" "$tmp/checkout/linux-repo" "$tmp/empty" DEADBEEF 2>"$tmp/empty.err"; then
    echo "linux-repo.sh: should reject an empty package directory" >&2
    return 1
  fi
  grep -q 'no .deb or .rpm' "$tmp/empty.err" || {
    echo "linux-repo.sh: a linux-repo/ checkout was refused for the wrong reason" >&2
    cat "$tmp/empty.err" >&2
    return 1
  }

  echo "linux-repo.sh: examples pass"
}

if [[ ${1:-} == --self-test ]]; then
  self_test
  exit
fi

repo=${1:?repository directory} pkgs=${2:?packages directory} key=${3:?gpg key id}
keep=${KEEP:-5}
url=https://sinhong2011.github.io/mypack

if marketing_site_path "$repo"; then
  echo "linux-repo.sh: refusing $repo; site/ is the marketing website" >&2
  exit 1
fi

shopt -s nullglob
debs=("$pkgs"/*.deb) rpms=("$pkgs"/*.rpm)
(( ${#debs[@]} && ${#rpms[@]} )) || { echo "linux-repo.sh: no .deb or .rpm in $pkgs" >&2; exit 1; }

mkdir -p "$repo/apt/pool/main/m/mypack" "$repo/rpm/packages"
cp -f "${debs[@]}" "$repo/apt/pool/main/m/mypack/"
cp -f "${rpms[@]}" "$repo/rpm/packages/"

# Keep the newest versions: one file per architecture per version.
prune() {
  local dir=$1 pattern=$2 glob=$3 versions old v
  versions=$(cd "$dir" && ls | sed -nE "s/$pattern/\1/p" | sort -uV)
  old=$(echo "$versions" | head -n -"$keep")
  for v in $old; do
    echo "dropping $v from $dir"
    rm -f "$dir"/${glob//VERSION/$v}
  done
}
prune "$repo/apt/pool/main/m/mypack" '^mypack_(.+)_[a-z0-9]+\.deb$' 'mypack_VERSION_*.deb'
prune "$repo/rpm/packages" '^mypack-(.+)-1\.[a-z0-9_]+\.rpm$' 'mypack-VERSION-1.*.rpm'

# APT: one suite, stable, with component main.
(
  cd "$repo/apt"
  rm -rf dists
  for arch in amd64 arm64; do
    d=dists/stable/main/binary-$arch
    mkdir -p "$d"
    apt-ftparchive --arch "$arch" packages pool >"$d/Packages"
    gzip -9kn "$d/Packages"
  done
  apt-ftparchive \
    -o APT::FTPArchive::Release::Origin=MyPack \
    -o APT::FTPArchive::Release::Label=MyPack \
    -o APT::FTPArchive::Release::Suite=stable \
    -o APT::FTPArchive::Release::Codename=stable \
    -o APT::FTPArchive::Release::Architectures="amd64 arm64" \
    -o APT::FTPArchive::Release::Components=main \
    release dists/stable >dists/stable/Release
  gpg --batch --yes --local-user "$key" --clearsign -o dists/stable/InRelease dists/stable/Release
  gpg --batch --yes --local-user "$key" --armor --detach-sign -o dists/stable/Release.gpg dists/stable/Release
)

# RPM: dnf, yum and zypper read the same repodata.
(
  cd "$repo/rpm"
  createrepo_c --update --quiet .
  gpg --batch --yes --local-user "$key" --armor --detach-sign -o repodata/repomd.xml.asc repodata/repomd.xml
  cat >mypack.repo <<EOF
[mypack]
name=MyPack
baseurl=$url/rpm
enabled=1
gpgcheck=0
repo_gpgcheck=1
gpgkey=$url/mypack.asc
EOF
)

gpg --batch --yes --armor --export "$key" >"$repo/mypack.asc"
gpg --batch --yes --export "$key" >"$repo/mypack.gpg"
touch "$repo/.nojekyll"
cat >"$repo/index.html" <<EOF
<!doctype html>
<meta charset="utf-8">
<title>MyPack packages</title>
<h1>MyPack package repositories</h1>
<p>APT and RPM repositories for <a href="https://github.com/sinhong2011/mypack">MyPack</a>.
Setup: <a href="https://github.com/sinhong2011/mypack#install">README › Install</a>.</p>
<ul>
<li><a href="apt/">apt/</a> (Debian, Ubuntu)</li>
<li><a href="rpm/">rpm/</a> (Fedora, RHEL, openSUSE)</li>
<li><a href="mypack.asc">mypack.asc</a>, <a href="mypack.gpg">mypack.gpg</a>: the signing key</li>
</ul>
EOF
echo "linux-repo.sh: $(ls "$repo/apt/pool/main/m/mypack" | wc -l | tr -d ' ') .deb, $(ls "$repo/rpm/packages" | wc -l | tr -d ' ') .rpm"
