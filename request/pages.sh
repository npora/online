#!/usr/bin/env bash
set -euo pipefail

site_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
mode="${1:-}"
checkout="${2:-}"
if [[ -z "$checkout" ]]; then
  printf 'Usage: bash request/pages.sh {check|preview} <candidate>\n' >&2
  printf '       bash request/pages.sh deploy <candidate> <pages-project> <branch>\n' >&2
  exit 2
fi

checkout="$(cd "$checkout" && pwd)"
revision="$(sed -n 's/^SOURCE_REV=//p' "$site_dir/source.env")"
if [[ "$(git -C "$checkout" rev-parse HEAD)" != "$revision" ]]; then
  printf 'Candidate revision does not match request/source.env\n' >&2
  exit 2
fi

export REQUEST_SOURCE_DIR="$checkout"
cd "$checkout/.online/request/website"
for file in dist/index.html dist/build.json dist/_routes.json dist/_headers functions/api.js wrangler.jsonc; do
  if [[ ! -f "$file" ]]; then
    printf 'Missing candidate file: %s\n' "$file" >&2
    exit 2
  fi
done
if ! node -e 'process.exit(Number(require("./dist/build.json").sourceRevision !== process.argv[1]))' "$revision"; then
  printf 'Built site revision does not match request/source.env. Rebuild the candidate first.\n' >&2
  exit 2
fi
if [[ ! -x node_modules/.bin/wrangler ]]; then
  printf 'Build the candidate with scripts/build-site.sh request first\n' >&2
  exit 2
fi

case "$mode" in
  check) exec npm run check ;;
  preview) exec npm run dev ;;
  deploy)
    project="${3:-}"
    branch="${4:-}"
    if [[ -z "$project" || -z "$branch" ]]; then
      printf 'Specify the existing Pages project and deployment branch explicitly\n' >&2
      exit 2
    fi
    exec node_modules/.bin/wrangler pages deploy dist --project-name "$project" \
      --branch "$branch" --commit-hash "$revision"
    ;;
  *) printf 'Unknown Pages operation: %s\n' "$mode" >&2; exit 2 ;;
esac
