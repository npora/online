#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
site="${1:-}"
case "$site" in
  vue|react|request) ;;
  *) printf 'Usage: %s {vue|react|request}\n' "$0" >&2; exit 2 ;;
esac

pin="$root/$site/source.env"
repo="$(sed -n 's/^SOURCE_REPO=//p' "$pin")"
revision="$(sed -n 's/^SOURCE_REV=//p' "$pin")"
if [[ ! "$repo" =~ ^https://github\.com/npora/[a-z-]+\.git$ || ! "$revision" =~ ^[0-9a-f]{40}$ ]]; then
  printf 'Invalid source pin for %s\n' "$site" >&2
  exit 2
fi

mkdir -p "$root/.build"
checkout="$(mktemp -d "$root/.build/$site.XXXXXX")"
git -C "$checkout" init --quiet
git -C "$checkout" fetch --quiet --depth=1 "$repo" "$revision"
git -C "$checkout" checkout --quiet --detach FETCH_HEAD
if [[ "$(git -C "$checkout" rev-parse HEAD)" != "$revision" ]]; then
  printf 'Fetched revision does not match %s\n' "$revision" >&2
  exit 1
fi

cd "$checkout"
case "$site" in
  vue)
    npm ci
    CLOUDFLARE_WORKER_NAME=npora-online-vue-candidate npm run build:cloudflare
    ;;
  react)
    npm ci
    VITE_DEMO_ENABLED=true VITE_API_PREFIX=/api npm run build
    ;;
  request)
    npm ci --prefix website --ignore-scripts
    npm run build --prefix website
    npm run check --prefix website
    ;;
esac
printf '\nCandidate checkout: %s\nPinned revision: %s\n' "$checkout" "$revision"
