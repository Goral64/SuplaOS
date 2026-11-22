#!/usr/bin/env bash
# Test the newest release of supla-docker with SuplaOS for real: a new
# install starts from it, and an install of SuplaOS 0.12.2 keeps its Supla
# until it is updated, as a whole, with its data.
#   tools/supla-release-test.sh <suplaos binary> <add-ons> <0.12.2 package>
# Needs Docker and the free ports 2015 and 2016. Not for a device in use:
# it takes the compose project and the container names of Supla.
set -euo pipefail

usage="usage: tools/supla-release-test.sh <suplaos binary> <add-ons> <0.12.2 package>"
bin=$(realpath "${1:?$usage}")
addons=$(realpath "${2:?$usage}")
deb=$(realpath "${3:?$usage}")

if [ -n "$(docker ps -aq --filter label=com.docker.compose.project=supla)" ]; then
  echo "Supla is installed here, the test would break it" >&2
  exit 1
fi
as_root() { if [ "$(id -u)" = 0 ]; then "$@"; else sudo "$@"; fi; }

before=$(mktemp -d)
dpkg-deb -x "$deb" "$before"

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

suplaos() { SUPLAOS_ROOT="$root" SUPLAOS_ADDONS="$addons" "$bin" "$@"; }
suplaos_before() {
  SUPLAOS_ROOT="$root" SUPLAOS_ADDONS="$before/usr/share/suplaos/addons" "$before/usr/bin/suplaos" "$@"
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

fail() {
  echo "$*" >&2
  docker logs --tail 20 supla-cloud >&2 || true
  exit 1
}

users() {
  docker exec supla-db sh -c 'exec mariadb -N -B --user=supla --password="$MYSQL_PASSWORD" supla -e "select count(*) from supla_user"'
}

images() { docker inspect --format '{{.Image}}' supla-cloud supla-server supla-db; }

echo "== New install"
root=$(mktemp -d)
suplaos install supla SUPLA_HOST_ADDRESS=127.0.0.1 || fail "the install failed"
answers
state=$(suplaos supla check)
echo "$state"
case "$(head -n 1 <<< "$state")" in
  *": waiting") echo "== The release waits for its images, nothing to test yet"; exit 0 ;;
  *": current") ;;
  *": available")
    # Only the database of the release may be left to update.
    grep -q "raised with the update" <<< "$state" || fail "a new install is not the newest release"
    suplaos supla upgrade || fail "the update of a new install failed"
    answers
    state=$(suplaos supla check)
    case "$(head -n 1 <<< "$state")" in
      *": current") ;;
      *) fail "a new install is not the release after its update" ;;
    esac
    ;;
  *) fail "a new install is not the newest release" ;;
esac
clean

echo "== An install of 0.12.2"
root=$(mktemp -d)
suplaos_before install supla SUPLA_HOST_ADDRESS=127.0.0.1 || fail "the install of 0.12.2 failed"
answers
suplaos_before run supla console supla:user:create test@example.com "test-$RANDOM-$RANDOM-Pw"
was=$(images)
suplaos apply || fail "applying the new SuplaOS failed"
answers
[ "$(images)" = "$was" ] || fail "an update of SuplaOS changed Supla"

echo "== Its update to the release"
suplaos supla upgrade || fail "the update of Supla failed"
answers
[ "$(users)" = 1 ] || fail "the data did not survive the update"
state=$(suplaos supla check)
case "$(head -n 1 <<< "$state")" in
  *": current") ;;
  *) fail "not the release after its update" ;;
esac
echo "== OK"
