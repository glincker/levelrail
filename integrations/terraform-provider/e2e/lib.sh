#!/usr/bin/env bash
# Shared drift-then-import flow. Sourced by e2e/<resource>/run.sh, never run directly.
# It needs a throwaway local instance and a built provider (see e2e/README.md).

e2e_die() { printf 'e2e: %s\n' "$*" >&2; exit 1; }

e2e_guard() {
  [ "${E2E_I_UNDERSTAND_THIS_USES_A_THROWAWAY_INSTANCE:-}" = "1" ] ||
    e2e_die "set E2E_I_UNDERSTAND_THIS_USES_A_THROWAWAY_INSTANCE=1 to confirm the instance can be wiped"
  : "${LEVELRAIL_ENDPOINT:?set LEVELRAIL_ENDPOINT to the local instance URL}"
  : "${LEVELRAIL_TOKEN:?set LEVELRAIL_TOKEN to a root token of that instance}"
  local rest host
  rest="${LEVELRAIL_ENDPOINT#*://}"
  rest="${rest%%/*}"
  case "$rest" in
    \[*) host="${rest%%]*}]" ;;
    *) host="${rest%%:*}" ;;
  esac
  case "$host" in
    localhost | 127.* | "[::1]") ;;
    *) e2e_die "endpoint host '$host' is not loopback, refusing to run" ;;
  esac
  TF="$(command -v tofu || command -v terraform || true)"
  [ -n "$TF" ] || e2e_die "neither tofu nor terraform is on PATH"
  command -v curl >/dev/null || e2e_die "curl is required"
  command -v jq >/dev/null || e2e_die "jq is required"
  export APP_API_URL="$LEVELRAIL_ENDPOINT" APP_API_TOKEN="$LEVELRAIL_TOKEN"
}

# e2e_api METHOD PATH [JSON_BODY]: an out-of-band call that bypasses the provider.
e2e_api() {
  local method="$1" path="$2" body="${3:-}"
  if [ -n "$body" ]; then
    curl -fsS -X "$method" -H "Authorization: Bearer $LEVELRAIL_TOKEN" -H 'Content-Type: application/json' \
      -d "$body" "$LEVELRAIL_ENDPOINT$path"
  else
    curl -fsS -X "$method" -H "Authorization: Bearer $LEVELRAIL_TOKEN" "$LEVELRAIL_ENDPOINT$path"
  fi
}

# e2e_plan_exit WANT LABEL: plan with -detailed-exitcode (0 empty, 2 diff) and compare.
e2e_plan_exit() {
  local want="$1" label="$2" code=0
  "$TF" plan -input=false -detailed-exitcode >plan.out 2>&1 || code=$?
  if [ "$code" != "$want" ]; then
    cat plan.out >&2
    e2e_die "$label: plan exit $code, wanted $want"
  fi
  printf 'ok: %s\n' "$label"
}

# e2e_flow ADDRESS: needs write_config, mutate and import_id defined by the caller.
e2e_flow() {
  local addr="$1" dir
  dir="$(mktemp -d)"
  cd "$dir" || e2e_die "cannot enter $dir"
  trap 'cd "$dir" && "$TF" destroy -auto-approve -input=false >/dev/null 2>&1; rm -rf "$dir"' EXIT
  write_config >main.tf
  "$TF" init -input=false >/dev/null 2>&1 || true
  "$TF" apply -auto-approve -input=false >apply.out 2>&1 || { cat apply.out >&2; e2e_die "first apply failed"; }
  e2e_plan_exit 0 "plan is empty after apply"
  mutate
  e2e_plan_exit 2 "plan shows the out of band change"
  "$TF" apply -auto-approve -input=false >apply.out 2>&1 || { cat apply.out >&2; e2e_die "re-apply failed"; }
  e2e_plan_exit 0 "plan is empty after re-apply"
  "$TF" state rm "$addr" >/dev/null
  "$TF" import -input=false "$addr" "$(import_id)" >import.out 2>&1 || { cat import.out >&2; e2e_die "import failed"; }
  e2e_plan_exit 0 "plan is empty after import into fresh state"
  printf 'PASS %s\n' "$addr"
}
