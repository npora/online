#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
site="$root/vue"
checkout="${1:-}"
if [[ ( "$#" -ne 1 && "$#" -ne 2 ) || ( "$#" -eq 2 && "$2" != "--deploy" ) ]]; then
  printf 'Usage: bash %s <Vue candidate checkout> [--deploy]\n' "$0" >&2
  exit 2
fi

if [[ "$checkout" == latest ]]; then
  checkout="$(cat "$root/.build/vue-current-path" 2>/dev/null || true)"
  if [[ -z "$checkout" ]]; then
    printf 'No successful Vue candidate is recorded. Run npm run build:vue first.\n' >&2
    exit 2
  fi
elif [[ "$checkout" != /* ]]; then
  checkout="$root/$checkout"
fi
checkout="$(cd "$checkout" && pwd -P)"
case "$checkout" in
  "$root"/.build/vue.*) ;;
  *) printf 'Pass the candidate checkout produced by scripts/build-site.sh vue.\n' >&2; exit 2 ;;
esac

revision="$(sed -n 's/^SOURCE_REV=//p' "$root/vue/source.env")"
if [[ "$(git -C "$checkout" rev-parse --show-toplevel)" != "$checkout" ||
      "$(git -C "$checkout" rev-parse HEAD)" != "$revision" ||
      ! -f "$checkout/dist/index.html" ]]; then
  printf 'Expected a built Vue candidate at the pinned revision.\n' >&2
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
