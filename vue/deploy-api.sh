#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if [[ $# -lt 1 || $# -gt 2 || ( $# -eq 2 && "$2" != "--deploy" ) ]]; then
  printf 'Usage: %s <Vue API candidate> [--deploy]\n' "$0" >&2
  exit 2
fi

candidate="$(cd "$1" && pwd -P)"
case "$candidate" in
  "$root"/.build/vue-api.*) ;;
  *) printf 'Pass the candidate produced by node vue/prepare-api.mjs.\n' >&2; exit 2 ;;
esac
if [[ ! -f "$candidate/worker.mjs" || ! -f "$candidate/wrangler.jsonc" || ! -f "$candidate/demo-seed.sql" ]]; then
  printf 'Vue API candidate is incomplete.\n' >&2
  exit 2
fi

args=(--config "$candidate/wrangler.jsonc" --no-autoconfig)
if [[ "${2:-}" != "--deploy" ]]; then
  args+=(--dry-run)
else
  if [[ ! "${AUTH_PEPPER:-}" =~ ^[0-9a-fA-F]{64}$ ]]; then
    printf 'Set the same 32-byte hexadecimal AUTH_PEPPER used to generate this candidate.\n' >&2
    exit 2
  fi
  secret_file="$(mktemp "${TMPDIR:-/tmp}/npora-vue-api-secrets.XXXXXX")"
  chmod 600 "$secret_file"
  trap 'rm -f "$secret_file"' EXIT
  printf 'AUTH_PEPPER=%s\n' "$AUTH_PEPPER" > "$secret_file"
  args+=(--secrets-file "$secret_file")
fi

cd "$root"
export WRANGLER_SEND_METRICS=false
npm exec --yes --package=wrangler@4.154.0 -- wrangler deploy "${args[@]}"
