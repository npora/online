#!/usr/bin/env bash
set -euo pipefail

site="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ $# -lt 1 || $# -gt 2 || ( $# -eq 2 && "$2" != "--deploy" ) ]]; then
  printf 'Usage: %s <candidate-checkout> [--deploy]\n' "$0" >&2
  exit 2
fi

checkout="$(cd "$1" && pwd)"
revision="$(sed -n 's/^SOURCE_REV=//p' "$site/source.env")"
if [[ "$(git -C "$checkout" rev-parse --show-toplevel)" != "$checkout" ||
      "$(git -C "$checkout" rev-parse HEAD)" != "$revision" ||
      ! -f "$checkout/dist/index.html" || ! -f "$checkout/dist/sw.js" ]]; then
  printf 'Expected a built React candidate at the pinned revision.\n' >&2
  exit 2
fi
if [[ "${2:-}" == "--deploy" ]]; then
  set --
else
  set -- --dry-run
fi

cd "$site"
export WRANGLER_SEND_METRICS=false
exec npm exec --yes --package=wrangler@4.154.0 -- wrangler deploy \
  --config "$site/wrangler.jsonc" --assets "$checkout/dist" --no-autoconfig "$@"
