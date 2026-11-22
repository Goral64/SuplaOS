#!/usr/bin/env bash
# Test a set of Supla for real: a new install, and an update of an install
# of the old set that keeps its data. supla-server has to answer on 2016.
#   tools/supla-set-test.sh <suplaos binary> <old add-ons> <new add-ons>
# Needs Docker and the free ports 2015 and 2016. Not for a device in use:
# it takes the compose project and the container names of Supla.
set -euo pipefail

usage="usage: tools/supla-set-test.sh <suplaos binary> <old add-ons> <new add-ons>"
bin=$(realpath "${1:?$usage}")
old=$(realpath "${2:?$usage}")
new=$(realpath "${3:?$usage}")

if [ -n "$(docker ps -aq --filter label=com.docker.compose.project=supla)" ]; then
  echo "Supla is installed here, the test would break it" >&2
  exit 1
fi
as_root() { if [ "$(id -u)" = 0 ]; then "$@"; else sudo "$@"; fi; }

root=
clean() {
  [ -n "$root" ] || return 0
  if [ -f "$root/stack/.env" ]; then
    (cd "$root/stack" && docker compose down -v --remove-orphans) >/dev/null 2>&1 || true
  fi
  as_root rm -rf "$root"
  root=
}
trap clean EXIT

suplaos() {
  local addons=$1
  shift
  SUPLAOS_ROOT="$root" SUPLAOS_ADDONS="$addons" "$bin" "$@"
}

answers() {
  for _ in $(seq 30); do
    if echo | timeout 5 openssl s_client -connect 127.0.0.1:2016 2>/dev/null | openssl x509 -noout >/dev/null 2>&1; then
      return 0
    fi
    sleep 4
  done
  echo "supla-server does not answer on 2016" >&2
  docker logs --tail 20 supla-server >&2 || true
  return 1
}

users() {
  docker exec supla-db sh -c 'exec mariadb -N -B --user=supla --password="$MYSQL_PASSWORD" supla -e "select count(*) from supla_user"'
}

echo "== New install"
root=$(mktemp -d)
suplaos "$new" install supla SUPLA_HOST_ADDRESS=127.0.0.1
answers
clean

echo "== Update from the current set"
root=$(mktemp -d)
suplaos "$old" install supla SUPLA_HOST_ADDRESS=127.0.0.1
answers
suplaos "$old" run supla console supla:user:create test@example.com "test-$RANDOM-$RANDOM-Pw"
before=$(users)
suplaos "$new" apply
answers
if [ "$before" != 1 ] || [ "$(users)" != 1 ]; then
  echo "the data did not survive the update" >&2
  exit 1
fi
echo "== OK"
