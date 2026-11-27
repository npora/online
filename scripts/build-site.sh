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

if [[ "$site" == vue || "$site" == react ]]; then
  mkdir -p "$checkout/public/database"
  cp "$root/shared/database/"* "$checkout/public/database/"
  printf '/public/database/\n' >> "$checkout/.git/info/exclude"
fi

cd "$checkout"
case "$site" in
  vue)
    npm ci
    VITE_AUTHENTICATOR_ENABLED=false VITE_API_PREFIX=/api npm run build
    NPORA_VUE_SOURCE="$checkout" node --test "$root/vue/cleanup/test/worker.test.mjs"
    ;;
  react)
    npm ci
    VITE_DEMO_ENABLED=true VITE_AUTHENTICATOR_ENABLED=false VITE_API_PREFIX=/api npm run build
    ;;
  request)
    site="$checkout/.online/request"
    mkdir -p "$site/website"
    cp "$pin" "$site/source.env"
    tar -C "$root/request/website" --exclude=node_modules --exclude=dist --exclude=.artifacts --exclude=.wrangler -cf - . |
      tar -C "$site/website" -xf -
    export REQUEST_SOURCE_DIR="$checkout"
    npm ci --prefix "$site/website" --ignore-scripts
    npm run build --prefix "$site/website"
    npm run check --prefix "$site/website"
    ;;
esac
printf '\nCandidate checkout: %s\nPinned revision: %s\n' "$checkout" "$revision"
