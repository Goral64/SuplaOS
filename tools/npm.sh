#!/usr/bin/env bash
# Run npm for the web interface in Docker, e.g. tools/npm.sh run build
# The npm cache is kept in .cache/npm.
set -euo pipefail

cd "$(dirname "$0")/.."
mkdir -p .cache/npm

exec docker run --rm -i \
  --user "$(id -u):$(id -g)" \
  --mount "type=bind,source=$PWD,destination=/src" \
  --workdir /src/web \
  -e npm_config_cache=/src/.cache/npm \
  -e npm_config_update_notifier=false \
  "${NODE_IMAGE:-node:22}" npm "$@"
