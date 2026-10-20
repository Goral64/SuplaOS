#!/usr/bin/env bash
# Run the Go toolchain in Docker, e.g. tools/go.sh test ./...
# Module and build caches are kept in .cache/go.
set -euo pipefail

cd "$(dirname "$0")/.."
mkdir -p .cache/go

exec docker run --rm -i \
  --user "$(id -u):$(id -g)" \
  --mount "type=bind,source=$PWD,destination=/src" \
  --workdir /src \
  -e GOPATH=/src/.cache/go/path -e GOCACHE=/src/.cache/go/build \
  -e GOOS -e GOARCH -e CGO_ENABLED=0 \
  "${GO_IMAGE:-golang:1.25-trixie}" go "$@"
