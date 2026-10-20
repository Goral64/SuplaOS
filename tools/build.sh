#!/usr/bin/env bash
# Build a SuplaOS image with debos running in Docker.
#   tools/build.sh amd64 [version]
# Images land in out/.
set -euo pipefail

cd "$(dirname "$0")/.."

target="${1:-}"
version="${2:-dev}"
recipe="image/suplaos-${target}.yaml"

if [ -z "$target" ] || [ ! -f "$recipe" ]; then
  echo "Usage: $0 amd64 [version]" >&2
  exit 1
fi

case "$target" in
  amd64) goarch=amd64 ;;
  *) goarch=arm64 ;;
esac

mkdir -p out

# suplaos CLI and the add-on catalog, put into the image by common.yaml.
staging="image/build/$goarch"
rm -rf "$staging"
mkdir -p "$staging/usr/bin" "$staging/usr/share/suplaos"
GOARCH="$goarch" tools/go.sh build -trimpath -ldflags="-s -w" -o "$staging/usr/bin/suplaos" ./cmd/suplaos
cp -r addons "$staging/usr/share/suplaos/addons"

debos_image="${DEBOS_IMAGE:-godebos/debos:latest}"
args=(--rm --tty --workdir /recipes
      --mount "type=bind,source=$PWD/image,destination=/recipes"
      --mount "type=bind,source=$PWD/out,destination=/out"
      --security-opt label=disable)

if [ -e /dev/kvm ]; then
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

if [ -e /dev/kvm ]; then
  docker run "${args[@]}" --entrypoint "${debos_cmd[0]}" "$debos_image" "${debos_cmd[@]:1}"
else
  # debos waits for udev to create partition devices, so start it first.
  docker run "${args[@]}" --entrypoint sh "$debos_image" -c \
    '/lib/systemd/systemd-udevd --daemon && exec "$@"' sh "${debos_cmd[@]}"
fi

if [ ! -e /dev/kvm ]; then
  # Files were created by root in the container.
  docker run --rm --mount "type=bind,source=$PWD/out,destination=/out" \
    --entrypoint chown "$debos_image" -R "$(id -u):$(id -g)" /out
fi

ls -lh out/
