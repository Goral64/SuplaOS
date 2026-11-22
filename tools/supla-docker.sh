#!/usr/bin/env bash
# Look for a newer set of Supla: supla-docker on its master branch and the
# images its compose file takes (latest of supla-cloud and supla-server,
# pinned by their version and digest, never an older version). When there
# is one, point the supla add-on at it, save the old and new versions of
# the supla-docker files SuplaOS uses into <dir>/old and <dir>/new, the
# add-on into <dir>/addon.yaml, the pull request text into <dir>/pr.md and
# print set=<title>. Prints nothing when up to date.
#   tools/supla-docker.sh <dir>
# Needs curl, jq and gh with a token (GH_TOKEN in GitHub Actions).
set -euo pipefail

dir=${1:?usage: tools/supla-docker.sh <dir>}
cd "$(dirname "$0")/.."
addon=addons/supla/addon.yaml
repo=SUPLA/supla-docker

old=$(grep -o "$repo/archive/[0-9a-f]*" "$addon" | sed 's|.*/||')
new=$(gh api "repos/$repo/commits/master" -q .sha)

# newest <image>: <version>@<digest> of the version tag that has the same
# amd64 and arm64 images as latest.
newest() {
  curl -fsSL "https://hub.docker.com/v2/repositories/supla/$1/tags?page_size=100" | jq -er '
    def platforms: [.images[] | select(.architecture == "amd64" or .architecture == "arm64")
      | [.architecture, .digest]] | sort;
    .results as $tags
    | ($tags[] | select(.name == "latest") | platforms) as $latest
    | [$tags[] | select((.name | test("^[0-9]+\\.[0-9]+(\\.[0-9]+)?$")) and platforms == $latest)]
    | max_by(.name | split(".") | map(tonumber))
    | select(. != null) | "\(.name)@\(.digest)"'
}

changed=()
title=()
for image in supla-cloud supla-server; do
  was=$(sed -n "s|^ *image: supla/$image:||p" "$addon")
  if ! now=$(newest "$image"); then
    echo "$image: no version tag for latest" >&2
    exit 1
  fi
  title+=("$image ${now%@*}")
  [ "$now" = "$was" ] && continue
  # A database migrated by a newer version does not work with an older one.
  if [ "$(printf '%s\n' "${was%@*}" "${now%@*}" | sort -V | tail -n 1)" != "${now%@*}" ]; then
    echo "$image: latest is ${now%@*}, older than ${was%@*} of the add-on" >&2
    exit 1
  fi
  changed+=("$image ${was%@*} -> ${now%@*}")
  sed -i "s|^\( *image: supla/$image:\).*|\1$now|" "$addon"
done

if [ "$old" = "$new" ] && [ ${#changed[@]} -eq 0 ]; then
  echo "Supla is up to date (supla-docker ${old:0:7})" >&2
  exit 0
fi

# The files of the fetch section, e.g. "  files: [docker-compose.yml, ...]".
files=$(sed -n 's/^  files: \[\(.*\)\]$/\1/p' "$addon" | tr -d ' ' | tr , ' ')
rm -rf "$dir"
mkdir -p "$dir/old" "$dir/new"
for f in $files; do
  curl -fsSL -o "$dir/old/$f" "https://raw.githubusercontent.com/$repo/$old/$f"
  curl -fsSL -o "$dir/new/$f" "https://raw.githubusercontent.com/$repo/$new/$f"
done
sed -i "s/$old/$new/" "$addon"
cp "$addon" "$dir/addon.yaml"

{
  echo "A new set of Supla: ${title[0]}, ${title[1]}, supla-docker [${new:0:7}](https://github.com/$repo/commit/$new)."
  echo
  for c in "${changed[@]}"; do
    echo "- $c"
  done
  if [ "$old" != "$new" ]; then
    echo
    echo "Commits of supla-docker since ${old:0:7}:"
    echo
    gh api "repos/$repo/compare/$old...$new" \
      -q '.commits[] | "- [" + .sha[0:7] + "](" + .html_url + ") " + (.commit.message | split("\n")[0])'
    echo
    if diff -q -r "$dir/old" "$dir/new" >/dev/null; then
      echo "The files SuplaOS uses ($files) didn't change."
    else
      echo "Changes in the files SuplaOS uses:"
      echo
      echo '```diff'
      (cd "$dir" && diff -ru old new || true) | sed -E 's/^(---|\+\+\+) ([^\t]*)\t.*/\1 \2/' | head -c 50000
      echo '```'
    fi
  fi
  echo
  echo "A new install and an update from the current set work on amd64 and arm64. Check it on a device before merging."
} > "$dir/pr.md"
echo "set=${title[0]}, ${title[1]}, supla-docker ${new:0:7}"
