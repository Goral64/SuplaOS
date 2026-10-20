#!/usr/bin/env bash
# Add suplaos packages to the apt repository in a directory (the gh-pages
# branch) and sign it. The signing key has to be in gpg already.
#   tools/apt-repo.sh <repository dir> <package.deb>...
# The last 5 versions of each architecture are kept.
set -euo pipefail

repo="$1"
shift
pool="$repo/pool/main/s/suplaos"
mkdir -p "$pool"
cp "$@" "$pool/"

for arch in amd64 arm64; do
  { ls "$pool"/suplaos_*_"$arch".deb 2>/dev/null || true; } | sort -V | head -n -5 | xargs -r rm -f
done

cd "$repo"
for arch in amd64 arm64; do
  dir="dists/trixie/main/binary-$arch"
  mkdir -p "$dir"
  apt-ftparchive --arch "$arch" packages pool > "$dir/Packages"
  gzip -9nkf "$dir/Packages"
done
apt-ftparchive \
  -o APT::FTPArchive::Release::Origin=SuplaOS \
  -o APT::FTPArchive::Release::Label=SuplaOS \
  -o APT::FTPArchive::Release::Suite=trixie \
  -o APT::FTPArchive::Release::Codename=trixie \
  -o APT::FTPArchive::Release::Architectures="amd64 arm64" \
  -o APT::FTPArchive::Release::Components=main \
  release dists/trixie > dists/trixie/Release
rm -f dists/trixie/InRelease dists/trixie/Release.gpg
gpg --batch --yes --clearsign -o dists/trixie/InRelease dists/trixie/Release
gpg --batch --yes -abs -o dists/trixie/Release.gpg dists/trixie/Release
