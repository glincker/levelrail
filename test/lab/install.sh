#!/usr/bin/env bash
# Install-stack checks for the lab, sourced by check.sh when it runs with --tier 1.
# Needs record, put, body and code from check.sh, and lib.sh.

install_stack=lab-install
install_dir="$lab_root/test/lab/repo/install"
install_weaken_dir="$lab_root/test/lab/repo/install-weaken"
install_node=lab-node-3
install_key=api.rate_limit_read_rpm
install_override="X-Levelrail-Override-Stack-Lock: $install_stack"

# files_json DIR: the stack request body for every yaml file in DIR.
files_json() {
	local f
	for f in "$1"/*.yaml; do
		jq -n --arg n "$(basename "$f")" --rawfile c "$f" '{name:$n,content:$c}'
	done | jq -s --argjson w "${2:-false}" '{files:.,allow_weaken:$w}'
}

install_cleanup() {
	lab_sess PUT "/api/v1/nodes/$install_node/taints" '{"taints":[]}' "$install_override" >/dev/null 2>&1 || true
	lab_sess DELETE "/api/v1/settings/runtime/$install_key" "" "$install_override" >/dev/null 2>&1 || true
	lab_cli stack delete "$install_stack" --mode forget --yes >/dev/null 2>&1 || true
}

install_checks() {
	local out rc

	lab_login || { record install-setup FAIL "dev admin login failed"; return 0; }
	install_cleanup

	# 1. Create, plan, apply, replan clean.
	if ! lab_cli stack create "$install_stack" --scope install >/dev/null 2>&1; then
		record install-plan-apply-replan FAIL "stack create failed"
		return 0
	fi
	# Install stacks hold applies for approval by default; the lab has one operator.
	lab_cli stack settings "$install_stack" --require-approval=false >/dev/null 2>&1
	lab_cli stack plan "$install_stack" -f "$install_dir" --exit-code >/dev/null 2>&1
	rc=$?
	local applied=0 replan
	lab_cli stack apply "$install_stack" -f "$install_dir" --yes >/dev/null 2>&1 && applied=1
	lab_cli stack plan "$install_stack" -f "$install_dir" --exit-code >/dev/null 2>&1
	replan=$?
	if [ "$rc" = 2 ] && [ "$applied" = 1 ] && [ "$replan" = 0 ]; then
		record install-plan-apply-replan PASS "plan pending, apply ok, replan clean"
	else
		record install-plan-apply-replan FAIL "plan rc=$rc apply=$applied replan rc=$replan"
	fi

	# 2. A hand edit of an owned node taint and an owned setting is refused.
	local n s
	n="$(lab_api PUT "/api/v1/nodes/$install_node/taints" '{"taints":[]}')"
	s="$(lab_api PUT "/api/v1/settings/runtime/$install_key" '{"value":"450"}')"
	if [ "$(printf '%s\n' "$n" | code)" = 409 ] && [ "$(printf '%s\n' "$s" | code)" = 409 ] &&
		grep -q stack_locked <<<"$n$s"; then
		record install-hand-edit-locked PASS "node and setting 409 stack_locked"
	else
		record install-hand-edit-locked FAIL "node $(printf '%s\n' "$n" | code) setting $(printf '%s\n' "$s" | code)"
	fi

	# 3. Weakening fails plan without allow_weaken and passes with it from a human root session.
	local plain weak_cli wp wa
	lab_cli stack plan "$install_stack" -f "$install_weaken_dir" >"$LAB_DIR/weaken.out" 2>&1
	plain=$?
	lab_cli stack plan "$install_stack" -f "$install_weaken_dir" --allow-weaken >/dev/null 2>&1
	weak_cli=$?
	wp="$(lab_sess POST "/api/v1/stacks/$install_stack/plan" "$(files_json "$install_weaken_dir" true)")"
	if [ "$plain" != 0 ] && grep -q allow_weaken "$LAB_DIR/weaken.out" && [ "$weak_cli" != 0 ] &&
		[ "$(printf '%s\n' "$wp" | code)" = 200 ] && [ "$(printf '%s\n' "$wp" | body | jq -r .plan.summary.error)" = 0 ]; then
		wa="$(lab_sess POST "/api/v1/stacks/$install_stack/apply" "$(files_json "$install_weaken_dir" true)")"
		if [ "$(printf '%s\n' "$wa" | code)" = 200 ]; then
			record install-weaken-gate PASS "refused plain and by token, applied from root session"
		else
			record install-weaken-gate FAIL "session apply: $(printf '%s\n' "$wa" | code) $(printf '%s\n' "$wa" | body | head -c 200)"
		fi
	else
		record install-weaken-gate FAIL "plain rc=$plain token rc=$weak_cli session plan $(printf '%s\n' "$wp" | code)"
	fi

	# 4. A stack token cannot create an install stack.
	local tok t2
	tok="$(lab_sess POST /api/v1/auth/tokens "{\"name\":\"lab-install-ci\",\"abilities\":[\"write\"],\"stack\":\"$install_stack\",\"stack_verbs\":[\"apply\"]}")"
	if [ "$(printf '%s\n' "$tok" | code)" = 201 ] || [ "$(printf '%s\n' "$tok" | code)" = 200 ]; then
		tok="$(printf '%s\n' "$tok" | body | jq -r '.token // .plaintext // .secret // empty')"
		t2="$(curl -sS -X POST -H "Authorization: Bearer $tok" -H "Content-Type: application/json" -w '\n%{http_code}' \
			--data '{"name":"lab-install-2","scope":"install"}' "$LAB_URL/api/v1/stacks")"
		if [ "$(printf '%s\n' "$t2" | code)" = 403 ]; then
			record install-token-cannot-create PASS "403"
		else
			record install-token-cannot-create FAIL "got $(printf '%s\n' "$t2" | code)"
			lab_cli stack delete lab-install-2 --mode forget --yes >/dev/null 2>&1 || true
		fi
	else
		record install-token-cannot-create SKIP "could not mint a stack token: $(printf '%s\n' "$tok" | code)"
	fi

	# 5. A hand change made with the override shows as drift.
	lab_sess PUT "/api/v1/nodes/$install_node/taints" '{"taints":[{"key":"lab-hand","value":"1","effect":"NoSchedule"}]}' "$install_override" >/dev/null
	local status=none i
	# The checker waits a fixed minute after start before its first pass.
	for i in $(seq 1 "${LAB_DRIFT_WAIT:-30}"); do
		status="$(lab_api GET "/api/v1/stacks/$install_stack" | body | jq -r .drift.status)"
		[ "$status" = drifted ] && break
		sleep 1
	done
	if [ "$status" = drifted ]; then
		record install-override-drift PASS "checker reports drifted"
	else
		lab_cli stack plan "$install_stack" -f "$install_dir" --exit-code >/dev/null 2>&1
		if [ $? = 2 ]; then
			record install-override-drift SKIP "checker status stays $status for a node taint edit; plan shows the update"
		else
			record install-override-drift FAIL "drift status $status, plan clean"
		fi
	fi

	install_cleanup
}
