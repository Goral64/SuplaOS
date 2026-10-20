#!/usr/bin/env bash
# Boot an amd64 SuplaOS image in QEMU (UEFI) for testing.
#   tools/run-qemu.sh [out/suplaos-dev-amd64.img]
# Forwards: ssh -> localhost:2222, panel -> localhost:8000, Supla Cloud -> https://localhost:8443.
# The image is not modified: QEMU writes to a temporary overlay (out/test.qcow2).
# Without qemu-system-x86_64 on the host, QEMU runs in a Debian container.
set -euo pipefail

cd "$(dirname "$0")/.."

image="${1:-out/suplaos-dev-amd64.img}"
[ -f "$image" ] || { echo "No image: $image" >&2; exit 1; }

qemu_args=(-m 2048 -smp 2 -nographic
  -drive "if=virtio,format=qcow2,file=out/test.qcow2"
  -nic "user,model=virtio-net-pci,hostfwd=tcp::2222-:22,hostfwd=tcp::8000-:80,hostfwd=tcp::8443-:443")

if [ -e /dev/kvm ]; then
  qemu_args+=(-enable-kvm -cpu host)
fi

if command -v qemu-system-x86_64 >/dev/null; then
  rm -f out/test.qcow2
  qemu-img create -q -f qcow2 -F raw -b "$(realpath "$image")" out/test.qcow2 8G
  exec qemu-system-x86_64 -bios /usr/share/ovmf/OVMF.fd "${qemu_args[@]}"
fi

docker build -q -t suplaos-qemu - >/dev/null <<'DOCKERFILE'
FROM debian:trixie-slim
RUN apt-get update && apt-get install -y --no-install-recommends qemu-system-x86 qemu-utils ovmf \
    && rm -rf /var/lib/apt/lists/*
DOCKERFILE

# Host network: forwarded connections then come from 127.0.0.1. With port
# publishing they would come from 172.17.0.1, which the guest routes to its
# own docker0 bridge.
docker_args=(--rm -it)
[ -e /dev/kvm ] && docker_args+=(--device /dev/kvm)

exec docker run "${docker_args[@]}" \
  --network host \
  -v "$PWD/out:/out" -w / \
  suplaos-qemu sh -c "rm -f /out/test.qcow2 && \
    qemu-img create -q -f qcow2 -F raw -b /out/$(basename "$image") /out/test.qcow2 8G && \
    exec qemu-system-x86_64 -bios /usr/share/ovmf/OVMF.fd $(printf '%q ' "${qemu_args[@]}" | sed 's|out/test.qcow2|/out/test.qcow2|')"
