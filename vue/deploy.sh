#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
action="${1:-}"
checkout="${2:-}"
if [[ "$#" -ne 2 || ( "$action" != check && "$action" != deploy ) ]]; then
  printf 'Usage: CLOUDFLARE_API_WORKER=<existing API Worker> bash %s {check|deploy} <Vue candidate checkout>\n' "$0" >&2
  exit 2
fi

checkout="$(cd "$checkout" && pwd -P)"
case "$checkout" in
  "$root"/.build/vue.*) ;;
  *) printf 'Pass the candidate checkout produced by scripts/build-site.sh vue.\n' >&2; exit 2 ;;
esac

revision="$(sed -n 's/^SOURCE_REV=//p' "$root/vue/source.env")"
if [[ "$(git -C "$checkout" rev-parse HEAD)" != "$revision" || -n "$(git -C "$checkout" status --porcelain)" ]]; then
  printf 'The Vue checkout must match the fixed source revision and have no changes.\n' >&2
  exit 2
fi

: "${CLOUDFLARE_API_WORKER:?Set CLOUDFLARE_API_WORKER to the verified production API Worker name}"
export CLOUDFLARE_WORKER_NAME=admin-demo
export CLOUDFLARE_API_WORKER
cd "$checkout"
npm run build:cloudflare
npm run check:cloudflare
if [[ "$action" == deploy ]]; then
  npm run deploy:cloudflare
fi
