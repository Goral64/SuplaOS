#!/usr/bin/env bash
# Look for a newer supla-docker on its master branch. When there is one,
# point the supla add-on at it, save the old and new versions of the files
# SuplaOS uses into <dir>/old and <dir>/new, the pull request text into
# <dir>/pr.md and print commit=<sha>. Prints nothing when up to date.
#   tools/supla-docker.sh <dir>
# Needs gh with a token (GH_TOKEN in GitHub Actions).
set -euo pipefail

dir=${1:?usage: tools/supla-docker.sh <dir>}
cd "$(dirname "$0")/.."
addon=addons/supla/addon.yaml
repo=SUPLA/supla-docker

old=$(grep -o "$repo/archive/[0-9a-f]*" "$addon" | sed 's|.*/||')
new=$(gh api "repos/$repo/commits/master" -q .sha)
if [ "$old" = "$new" ]; then
  echo "supla-docker is up to date (${old:0:7})" >&2
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

{
  echo "supla-docker master is now at [${new:0:7}](https://github.com/$repo/commit/$new)."
  echo
  echo "Commits since ${old:0:7}:"
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
  echo
  echo "The tests passed with the new files. Check it on a device before merging."
} > "$dir/pr.md"
echo "commit=$new"
