#!/usr/bin/env bash
# Run an amd64 SuplaOS image in Hyper-V (from WSL).
#   tools/hyperv.sh [out/suplaos-dev-amd64.img]
# The image is converted to VHDX in $SUPLAOS_HYPERV_DIR (Windows path,
# default F:\Hyper-V\SuplaOS) and the SuplaOS-dev VM is created or gets
# the new disk. The VM disk is replaced every time, so its data is lost.
set -euo pipefail

cd "$(dirname "$0")/.."

image="${1:-out/suplaos-dev-amd64.img}"
win_dir="${SUPLAOS_HYPERV_DIR:-F:\\Hyper-V\\SuplaOS}"
powershell=/mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe

[ -f "$image" ] || { echo "No image: $image" >&2; exit 1; }

dir="$(wslpath -u "$win_dir")"
mkdir -p "$dir"

new_disk=suplaos-dev.new.vhdx
echo "Converting $image to $win_dir\\$new_disk"
if command -v qemu-img >/dev/null; then
  qemu-img convert -p -O vhdx -o subformat=dynamic "$image" "$dir/$new_disk"
else
  docker build -q -t suplaos-qemu - >/dev/null <<'DOCKERFILE'
FROM debian:trixie-slim
RUN apt-get update && apt-get install -y --no-install-recommends qemu-system-x86 qemu-utils ovmf \
    && rm -rf /var/lib/apt/lists/*
DOCKERFILE
  docker run --rm \
    -v "$(realpath "$(dirname "$image")"):/in:ro" -v "$dir:/out" \
    suplaos-qemu qemu-img convert -O vhdx -o subformat=dynamic \
    "/in/$(basename "$image")" "/out/$new_disk"
fi

# Run the script from the Windows disk: an elevated PowerShell may not see
# the \\wsl$ share.
cp tools/hyperv.ps1 "$dir/hyperv.ps1"
"$powershell" -NoProfile -ExecutionPolicy Bypass -File "$win_dir\\hyperv.ps1" \
  -Dir "$win_dir" -NewDisk "$new_disk"
