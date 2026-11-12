#!/usr/bin/env bash
# Adds a release's Debian and RPM packages to the signed APT and RPM
# repositories that GitHub Pages serves from the gh-pages branch, at
# https://sinhong2011.github.io/mypack/. The Release workflow runs it on
# Ubuntu (apt-utils, createrepo-c, gnupg). See docs/releasing.md.
#
#   scripts/linux-repo.sh SITE PACKAGES_DIR GPG_KEY_ID
#
# SITE is a checkout of gh-pages; PACKAGES_DIR holds the new .deb and .rpm
# files. The newest $KEEP versions are kept (default 5).
set -euo pipefail

site=${1:?site directory} pkgs=${2:?packages directory} key=${3:?gpg key id}
keep=${KEEP:-5}
url=https://sinhong2011.github.io/mypack

shopt -s nullglob
debs=("$pkgs"/*.deb) rpms=("$pkgs"/*.rpm)
(( ${#debs[@]} && ${#rpms[@]} )) || { echo "linux-repo.sh: no .deb or .rpm in $pkgs" >&2; exit 1; }

mkdir -p "$site/apt/pool/main/m/mypack" "$site/rpm/packages"
cp -f "${debs[@]}" "$site/apt/pool/main/m/mypack/"
cp -f "${rpms[@]}" "$site/rpm/packages/"

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
prune "$site/apt/pool/main/m/mypack" '^mypack_(.+)_[a-z0-9]+\.deb$' 'mypack_VERSION_*.deb'
prune "$site/rpm/packages" '^mypack-(.+)-1\.[a-z0-9_]+\.rpm$' 'mypack-VERSION-1.*.rpm'

# APT: one suite, stable, with component main.
(
  cd "$site/apt"
  rm -rf dists
  for arch in amd64 arm64; do
    d=dists/stable/main/binary-$arch
    mkdir -p "$d"
    apt-ftparchive --arch "$arch" packages pool >"$d/Packages"
    gzip -9kn "$d/Packages"
  done
  apt-ftparchive \
    -o APT::FTPArchive::Release::Origin=Mypack \
    -o APT::FTPArchive::Release::Label=Mypack \
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
  cd "$site/rpm"
  createrepo_c --update --quiet .
  gpg --batch --yes --local-user "$key" --armor --detach-sign -o repodata/repomd.xml.asc repodata/repomd.xml
  cat >mypack.repo <<EOF
[mypack]
name=Mypack
baseurl=$url/rpm
enabled=1
gpgcheck=0
repo_gpgcheck=1
gpgkey=$url/mypack.asc
EOF
)

gpg --batch --yes --armor --export "$key" >"$site/mypack.asc"
gpg --batch --yes --export "$key" >"$site/mypack.gpg"
touch "$site/.nojekyll"
cat >"$site/index.html" <<EOF
<!doctype html>
<meta charset="utf-8">
<title>Mypack packages</title>
<h1>Mypack package repositories</h1>
<p>APT and RPM repositories for <a href="https://github.com/sinhong2011/mypack">Mypack</a>.
Setup: <a href="https://github.com/sinhong2011/mypack#install">README › Install</a>.</p>
<ul>
<li><a href="apt/">apt/</a> (Debian, Ubuntu)</li>
<li><a href="rpm/">rpm/</a> (Fedora, RHEL, openSUSE)</li>
<li><a href="mypack.asc">mypack.asc</a>, <a href="mypack.gpg">mypack.gpg</a>: the signing key</li>
</ul>
EOF
echo "linux-repo.sh: $(ls "$site/apt/pool/main/m/mypack" | wc -l | tr -d ' ') .deb, $(ls "$site/rpm/packages" | wc -l | tr -d ' ') .rpm"
