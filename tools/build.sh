#!/usr/bin/env bash
# Build a SuplaOS image with debos running in Docker.
#   tools/build.sh amd64|rpi [version]
# Images land in out/.
set -euo pipefail

cd "$(dirname "$0")/.."

target="${1:-}"
version="${2:-dev}"
recipe="image/suplaos-${target}.yaml"

if [ -z "$target" ] || [ ! -f "$recipe" ]; then
  echo "Usage: $0 amd64|rpi [version]" >&2
  exit 1
fi

case "$target" in
  amd64) goarch=amd64 ;;
  *) goarch=arm64 ;;
esac

mkdir -p out

# The suplaos package (panel with the web interface, suplaos command,
# add-on catalog), installed into the image by common.yaml. A copy lands in
# out/ for the apt repository.
tools/npm.sh ci --no-audit --no-fund
tools/npm.sh run build
staging="image/build/$goarch"
rm -rf "$staging"
mkdir -p "$staging/tmp"
rm -f out/suplaos_*_"$goarch".deb
tools/package.sh "$goarch" "$version" out
cp out/suplaos_*_"$goarch".deb "$staging/tmp/suplaos.deb"

debos_image="${DEBOS_IMAGE:-godebos/debos:latest}"
args=(--rm --workdir /recipes
      --mount "type=bind,source=$PWD/image,destination=/recipes"
      --mount "type=bind,source=$PWD/out,destination=/out"
      --security-opt label=disable)

[ -t 1 ] && args+=(--tty)

# With KVM debos builds in its own small VM (fakemachine). SUPLAOS_NO_KVM=1
# forces the privileged container way, used without KVM (e.g. WSL).
use_kvm=
if [ -e /dev/kvm ] && [ -z "${SUPLAOS_NO_KVM:-}" ]; then
  use_kvm=1
fi

if [ -n "$use_kvm" ]; then
  args+=(--device /dev/kvm --user "$(id -u)")
  debos_args=()
else
  # No KVM (e.g. WSL): run debos directly in a privileged container instead
  # of its fakemachine VM.
  args+=(--privileged --volume /dev:/dev)
  debos_args=(--disable-fakemachine)
fi

debos_cmd=(debos "${debos_args[@]}"
  --artifactdir=/out
  --template-var="version:${version}"
  --memory=4G --cpus="$(nproc)"
  "$(basename "$recipe")")

if [ -n "$use_kvm" ]; then
  docker run "${args[@]}" --entrypoint "${debos_cmd[0]}" "$debos_image" "${debos_cmd[@]:1}"
else
  # debos waits for udev to create partition devices, so start it first.
  # An arm64 image built on x86-64 runs its programs with qemu: register it
  # for the build (binfmt is global for the kernel) and remove it after.
  docker run "${args[@]}" -e GOARCH="$goarch" --entrypoint sh "$debos_image" -c '
    set -e
    registered=
    if [ "$GOARCH" = arm64 ] && [ "$(uname -m)" != aarch64 ]; then
      mountpoint -q /proc/sys/fs/binfmt_misc ||
        mount -t binfmt_misc binfmt_misc /proc/sys/fs/binfmt_misc
      if [ ! -e /proc/sys/fs/binfmt_misc/qemu-aarch64 ]; then
        cat /usr/lib/binfmt.d/qemu-aarch64.conf > /proc/sys/fs/binfmt_misc/register
        registered=1
      fi
    fi
    /lib/systemd/systemd-udevd --daemon
    rc=0
    "$@" || rc=$?
    if [ -n "$registered" ]; then echo -1 > /proc/sys/fs/binfmt_misc/qemu-aarch64; fi
    exit $rc' sh "${debos_cmd[@]}"
fi

if [ -z "$use_kvm" ]; then
  # Files were created by root in the container.
  docker run --rm --mount "type=bind,source=$PWD/out,destination=/out" \
    --entrypoint chown "$debos_image" -R "$(id -u):$(id -g)" /out
fi

ls -lh out/
