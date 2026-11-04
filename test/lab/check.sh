#!/usr/bin/env bash
# Scheduler checks against the running lab (tier 1, store-seeded nodes, no agents).
# Everything goes through the real CLI and REST API. Exits non-zero and names each failed check.
#   test/lab/up.sh && test/lab/check.sh [--tier 1]
# --tier 1 adds the install stack group (needs the dev-mode admin and a fast drift checker, see up.sh).
set -uo pipefail
tier=0
[ "${1:-}" = "--tier" ] && tier="${2:-0}"
# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
export APP_EXPERIMENTAL=iac

results=()
failed=()
record() { # name status detail
	results+=("$(printf '%-34s %-5s %s' "$1" "$2" "$3")")
	[ "$2" = FAIL ] && failed+=("$1")
	return 0
}

body() { sed '$d'; }
code() { tail -n1; }

# put PATH JSON: expects a 2xx.
put() {
	local out
	out="$(lab_api PUT "$1" "$2")"
	case "$(printf '%s\n' "$out" | code)" in 2??) return 0 ;; *) echo "PUT $1: $out" >&2; return 1 ;; esac
}
preview() { lab_api GET "/api/v1/apps/$1/placement/decision" | body; }
eligible() { jq -r --arg n "$2" '.preview.considered[] | select(.node_id==$n) | .eligible' <<<"$1"; }

reset_rules() {
	put /api/v1/nodes/lab-node-2/taints '{"taints":[]}' || true
	put /api/v1/apps/lab-web-a/placement '{"tolerations":[],"affinity":{}}' || true
	lab_api DELETE /api/v1/settings/runtime/placement.score_mode >/dev/null || true
}

if ! lab_cli apply --yes -f "$lab_root/test/lab/repo/apps.yaml" >/dev/null 2>&1; then
	echo "lab: could not apply the fixture apps, is the lab up?" >&2
	exit 1
fi
reset_rules

# 1. A NoSchedule taint filters the node.
put /api/v1/nodes/lab-node-2/taints '{"taints":[{"key":"lab","value":"gpu","effect":"NoSchedule"}]}'
p="$(preview lab-web-a)"
if [ "$(eligible "$p" lab-node-2)" = false ] && [ "$(jq -r .preview.node_id <<<"$p")" != lab-node-2 ]; then
	record taint-noschedule-filters PASS "node-2 ineligible"
else
	record taint-noschedule-filters FAIL "node-2 still eligible"
fi

# 2. A matching toleration lets the node back in.
put /api/v1/apps/lab-web-a/placement '{"tolerations":[{"key":"lab","operator":"Exists","effect":"NoSchedule"}]}'
p="$(preview lab-web-a)"
if [ "$(eligible "$p" lab-node-2)" = true ]; then
	record toleration-allows PASS "node-2 eligible"
else
	record toleration-allows FAIL "node-2 still filtered"
fi
reset_rules

# 3. Required anti-affinity keeps the app off the node that runs its peer.
peer="$(lab_api GET /api/v1/apps/lab-web-b/placement/decision | body | jq -r .node_id)"
put /api/v1/apps/lab-web-a/placement '{"affinity":{"apart":[{"match":[{"key":"name","op":"In","values":["lab-web-b"]}],"topology":"node"}]}}'
p="$(preview lab-web-a)"
chosen="$(jq -r .preview.node_id <<<"$p")"
if [ -n "$peer" ] && [ "$peer" != null ] && [ "$(eligible "$p" "$peer")" = false ] && [ "$chosen" != "$peer" ]; then
	record anti-affinity-spreads PASS "peer on $peer, a on $chosen"
else
	record anti-affinity-spreads FAIL "peer $peer, chosen $chosen"
fi
reset_rules

# 4. Bin-pack vs spread: the setting flips live. Seeded nodes report no capacity, so the
# headroom ordering that differs between the modes needs real agents (tier 2).
put /api/v1/settings/runtime/placement.score_mode '{"value":"binpack"}'
bin="$(preview lab-web-a | jq -r .preview.policy.score_mode)"
lab_api DELETE /api/v1/settings/runtime/placement.score_mode >/dev/null
spr="$(preview lab-web-a | jq -r .preview.policy.score_mode)"
if [ "$bin" = binpack ] && [ "$spr" = spread ]; then
	record binpack-vs-spread-setting PASS "policy flips live, ordering needs tier 2"
else
	record binpack-vs-spread-setting FAIL "binpack=$bin spread=$spr"
fi

# 5. A watch resumed from a version older than a restart gets 410.
rv="$(lab_api GET /api/v2/apps | body | jq -r .metadata.resourceVersion)"
"$(dirname "${BASH_SOURCE[0]}")/down.sh" >/dev/null && "$(dirname "${BASH_SOURCE[0]}")/up.sh" >/dev/null
status="$(curl -sS -o /dev/null -m 5 -w '%{http_code}' -H "Authorization: Bearer $(lab_token)" "$LAB_URL/api/v2/apps?watch=true&resourceVersion=$rv")"
if [ "$status" = 410 ]; then
	record watch-410-after-restart PASS "rv $rv -> 410"
else
	record watch-410-after-restart FAIL "rv $rv -> $status"
fi

# 6. Install stack checks (opt in with --tier 1).
if [ "$tier" = 1 ]; then
	# shellcheck source=install.sh
	. "$(dirname "${BASH_SOURCE[0]}")/install.sh"
	install_checks
fi

echo
printf '%-34s %-5s %s\n' CHECK RESULT DETAIL
printf '%s\n' "${results[@]}"
if [ "${#failed[@]}" -gt 0 ]; then
	echo
	echo "lab: failed: ${failed[*]}" >&2
	exit 1
fi
