#!/usr/bin/env bash
# Build the suplaos Debian package: panel, suplaos command, add-on catalog
# and system files from package/.
#   tools/package.sh amd64|arm64 [version] [output dir]
# The web interface has to be built first (tools/npm.sh run build).
set -euo pipefail

cd "$(dirname "$0")/.."

arch="${1:-}"
version="${2:-dev}"
out="${3:-out}"
case "$arch" in
  amd64|arm64) ;;
  *) echo "Usage: $0 amd64|arm64 [version] [output dir]" >&2; exit 1 ;;
esac

# Debian versions start with a digit; development builds sort before any
# release.
if [[ ! "$version" =~ ^[0-9] ]]; then
  version="0~${version//-/.}+$(date -u +%Y%m%d%H%M)"
fi

root=".cache/package/$arch"
rm -rf "$root"
mkdir -p "$root/usr/bin" "$root/usr/share/suplaos" "$out"
cp -r package/. "$root/"
for cmd in suplaos suplaos-panel; do
  GOARCH="$arch" tools/go.sh build -trimpath -ldflags="-s -w" -o "$root/usr/bin/$cmd" "./cmd/$cmd"
done
cp -r addons "$root/usr/share/suplaos/addons"
echo "$version" > "$root/usr/share/suplaos/version"

recommends=""
if [ "$arch" = arm64 ]; then
  # gpiomon reads the power button of an Argon ONE case.
  recommends=$'\nRecommends: gpiod'
fi
# Firmware of common Wi-Fi cards in x86-64 computers, so that it comes with
# an update too. The Raspberry Pi image has its own.
if [ "$arch" = amd64 ]; then
  recommends=$'\nRecommends: firmware-iwlwifi, firmware-atheros, firmware-realtek'
fi

cat > "$root/DEBIAN/control" <<CONTROL
Package: suplaos
Version: $version
Architecture: $arch
Maintainer: Goral64 <goral64@gmail.com>
Depends: docker.io, docker-cli, docker-compose, network-manager, wpasupplicant, wireless-regdb, iw, avahi-daemon, cloud-guest-utils, e2fsprogs, ca-certificates, cifs-utils, nfs-common$recommends
Section: admin
Priority: optional
Homepage: https://github.com/Goral64/SuplaOS
Description: SuplaOS panel and add-ons
 Web panel and suplaos command of SuplaOS: your own Supla server with
 add-ons (Mosquitto, Zigbee2MQTT, zigbee2supla) on one device.
 .
 Part of the SuplaOS images, not meant for installing on an existing
 Debian system.
CONTROL

# Files in /etc are configuration: dpkg keeps local changes on updates.
(cd "$root" && find etc -type f | sed 's|^|/|') > "$root/DEBIAN/conffiles"
chmod -R u=rwX,go=rX "$root"

docker run --rm --user "$(id -u):$(id -g)" \
  --mount "type=bind,source=$PWD,destination=/src" --workdir /src \
  debian:trixie-slim dpkg-deb --root-owner-group -Zxz --build "$root" "$out/suplaos_${version}_${arch}.deb"
