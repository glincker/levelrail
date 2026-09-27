#!/usr/bin/env bash
# End-to-end smoke test against an already-provisioned Ubuntu VPS: install,
# first admin, git deploys of three fixtures, TLS, rollback, then an upgrade
# under continuous request load. Never provisions or destroys cloud
# resources; it only drives SSH, the CLI, and the HTTP API.
#
# Required env:
#   SMOKE_HOST         [user@]host reachable by ssh (root or passwordless sudo)
#   SMOKE_DOMAIN_BASE  domain whose wildcard A record points at SMOKE_HOST
#   SMOKE_GH_TOKEN     GitHub token with contents:write on SMOKE_REPO
#   SMOKE_REPO         owner/name of a scratch repo the run pushes a branch to
#   SMOKE_FROM_TAG     release tag installed first
#   SMOKE_TO_TAG       release tag upgraded to under load
# Optional env:
#   SMOKE_SSH_OPTS     extra ssh options
#   SMOKE_CLI          path to a levelrail-cli binary (default: build from this tree)
#   SMOKE_WAIT         per-wait timeout in seconds (default 600)
#
# Usage: scripts/vps-smoke.sh [--dry-run]

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
dry_run=0
case "${1:-}" in
--dry-run) dry_run=1 ;;
"") ;;
*)
	echo "usage: $0 [--dry-run]" >&2
	exit 2
	;;
esac

apps=(smoke-nextjs smoke-goapi smoke-static)
app_dir() {
	case "$1" in
	smoke-nextjs) echo nextjs ;;
	smoke-goapi) echo goapi ;;
	smoke-static) echo static ;;
	*) return 1 ;;
	esac
}
app_port() {
	case "$1" in
	smoke-nextjs) echo 3000 ;;
	*) echo 8080 ;;
	esac
}

wait_secs="${SMOKE_WAIT:-600}"
run_id="$(date +%s)"
branch="smoke/${run_id}"
data_dir="/var/lib/levelrail-data"

work="" host_addr="" api="" cli_bin="" token="" load_pids=()
results=()
failed=0

log() { printf '[smoke] %s\n' "$*" >&2; }

record() {
	results+=("$(printf '%-34s %-5s %s' "$1" "$2" "${3:-}")")
	[ "$2" = PASS ] || failed=1
}

# shellcheck disable=SC2086
remote() { ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new ${SMOKE_SSH_OPTS:-} "$SMOKE_HOST" "$@"; }

remote_root() {
	remote "s=; [ \"\$(id -u)\" -eq 0 ] || s='sudo -n'; \$s $*"
}

domain_of() { echo "${1}.${SMOKE_DOMAIN_BASE}"; }

cli() { APP_API_URL="$api" APP_API_TOKEN="$token" "$cli_bin" "$@"; }

# poll CMD... until it succeeds or wait_secs elapse.
poll() {
	local deadline=$((SECONDS + wait_secs))
	until "$@"; do
		[ "$SECONDS" -lt "$deadline" ] || return 1
		sleep 3
	done
}

body_has() { # domain marker
	[ "$(curl -fsS --max-time 8 "https://$1/" 2>/dev/null | tr -d '[:space:]')" = "$2" ]
}

latest_deploy_id() { cli apps deploys list "$1" --json | jq -r '.[0].id // ""'; }

deploy_settled_after() { # app previous-id
	local id status
	id="$(latest_deploy_id "$1")"
	[ -n "$id" ] && [ "$id" != "$2" ] || return 1
	status="$(cli apps deploys list "$1" --json | jq -r '.[0].status')"
	case "$status" in
	succeeded) return 0 ;;
	failed | cancelled | canceled)
		log "$1 deploy $id ended as $status"
		DEPLOY_FAILED=1
		return 0
		;;
	*) ;;
	esac
	return 1
}
DEPLOY_FAILED=0

wait_deploy() { # app previous-id
	DEPLOY_FAILED=0
	poll deploy_settled_after "$1" "$2" || return 1
	[ "$DEPLOY_FAILED" -eq 0 ]
}

trigger_build() { # app
	local dir
	dir="$(app_dir "$1")"
	cli apps builds trigger "$1" --repo "https://github.com/${SMOKE_REPO}" --ref "$branch" \
		--image-repo "smoke/$1" --base-directory "$dir" >/dev/null
}

push_marker() { # value
	local a
	for a in "${apps[@]}"; do
		echo "$1" >"$work/repo/$(app_dir "$a")/MARKER"
	done
	git -C "$work/repo" add -A
	git -C "$work/repo" -c user.name=smoke -c user.email=smoke@localhost commit -q -m "smoke: marker $1"
	git -C "$work/repo" push -q origin "HEAD:refs/heads/$branch"
}

stop_load() {
	[ -e "$work/stop" ] || : >"$work/stop"
	local p
	for p in "${load_pids[@]:-}"; do
		[ -z "$p" ] || wait "$p" 2>/dev/null || true
	done
	load_pids=()
}

cleanup() {
	local rc=$?
	[ -z "$work" ] || stop_load
	if [ -n "$work" ] && [ -d "$work/repo/.git" ]; then
		git -C "$work/repo" push -q origin --delete "$branch" >/dev/null 2>&1 || true
	fi
	[ -z "$work" ] || rm -rf "$work"
	return "$rc"
}

step_prereqs() {
	local t
	for t in ssh curl jq openssl git; do
		command -v "$t" >/dev/null || {
			log "missing tool: $t"
			return 1
		}
	done
	if [ -n "${SMOKE_CLI:-}" ]; then
		cli_bin="$SMOKE_CLI"
	else
		command -v go >/dev/null || {
			log "set SMOKE_CLI or install go"
			return 1
		}
		cli_bin="$work/levelrail-cli"
		(cd "$repo_root" && go build -o "$cli_bin" ./cmd/levelrail-cli) || return 1
	fi
	remote true
}

step_install() {
	remote_root env LEVELRAIL_VERSION="$SMOKE_FROM_TAG" LEVELRAIL_SKIP_REACHABILITY=1 sh -s <"$repo_root/install.sh" >"$work/install.log" 2>&1 || {
		tail -n 30 "$work/install.log" >&2
		return 1
	}
	poll curl -fsS --max-time 5 -o /dev/null "$api/api/v1/auth/setup-status"
}

step_admin() {
	local setup_token password cookie
	setup_token="$(remote_root env APP_DATA_DIR="$data_dir" /usr/local/bin/levelrail setup-token | tr -d '[:space:]')"
	[ -n "$setup_token" ] || return 1
	password="$(openssl rand -hex 16)"
	printf '%s' "$password" >"$work/admin-password"
	curl -fsS -D "$work/register.hdr" -o /dev/null -X POST "$api/api/v1/auth/register" \
		-H 'Content-Type: application/json' \
		-d "$(jq -n --arg u admin@smoke.invalid --arg p "$password" --arg t "$setup_token" '{username:$u,password:$p,setup_token:$t}')" || return 1
	cookie="$(tr -d '\r' <"$work/register.hdr" | sed -n 's/^[Ss]et-[Cc]ookie: \(session_token=[^;]*\).*/\1/p' | head -n1)"
	[ -n "$cookie" ] || return 1
	# The session cookie is Secure, so it is replayed by hand over plain http.
	token="$(curl -fsS -X POST "$api/api/v1/auth/tokens" -H "Cookie: $cookie" -H 'Content-Type: application/json' \
		-d '{"name":"vps-smoke","abilities":["root"]}' | jq -r '.token')"
	[ -n "$token" ] && [ "$token" != null ]
}

step_seed_repo() {
	local a
	mkdir -p "$work/repo"
	git -C "$work/repo" init -q -b main
	git -C "$work/repo" remote add origin "https://x-access-token:${SMOKE_GH_TOKEN}@github.com/${SMOKE_REPO}.git"
	for a in "${apps[@]}"; do
		cp -R "$repo_root/test/fixtures/smoke/$(app_dir "$a")" "$work/repo/"
	done
	push_marker v1
}

step_connect_github() {
	local a
	for a in "${apps[@]}"; do
		cli apps create --name "$a" --image busybox:1.36 --port "$(app_port "$a")" --yes >/dev/null || return 1
		cli apps git-source set "$a" --repo-url "https://github.com/${SMOKE_REPO}" --branch "$branch" \
			--build-type dockerfile --build-path "$(app_dir "$a")/Dockerfile" --token-secret "$SMOKE_GH_TOKEN" >/dev/null || return 1
	done
}

step_deploy_fixtures() {
	local a prev
	for a in "${apps[@]}"; do
		cli apps domains add "$a" "$(domain_of "$a")" >/dev/null || return 1
		prev="$(latest_deploy_id "$a")"
		trigger_build "$a" || return 1
		wait_deploy "$a" "$prev" || return 1
	done
	for a in "${apps[@]}"; do
		poll body_has "$(domain_of "$a")" v1 || {
			log "$a did not serve marker v1"
			return 1
		}
		curl -fsS --max-time 8 -o /dev/null "https://$(domain_of "$a")/healthz" || return 1
	done
}

tls_ok() { # domain
	local d="$1" out end
	out="$(openssl s_client -connect "${host_addr}:443" -servername "$d" -verify_hostname "$d" -verify_return_error </dev/null 2>&1)" || {
		log "$d: chain or hostname verification failed"
		return 1
	}
	grep -q 'Verify return code: 0 (ok)' <<<"$out" || return 1
	end="$(openssl s_client -connect "${host_addr}:443" -servername "$d" </dev/null 2>/dev/null | openssl x509 -noout -enddate | cut -d= -f2)"
	log "$d: notAfter=$end"
	openssl s_client -connect "${host_addr}:443" -servername "$d" </dev/null 2>/dev/null | openssl x509 -noout -checkend $((7 * 86400)) >/dev/null
}

step_tls() {
	local a bad=0
	for a in "${apps[@]}"; do
		tls_ok "$(domain_of "$a")" || bad=1
	done
	[ "$bad" -eq 0 ]
}

step_rollback() {
	local a prev target
	push_marker v2
	for a in "${apps[@]}"; do
		prev="$(latest_deploy_id "$a")"
		trigger_build "$a" || return 1
		wait_deploy "$a" "$prev" || return 1
	done
	for a in "${apps[@]}"; do
		poll body_has "$(domain_of "$a")" v2 || {
			log "$a did not serve marker v2"
			return 1
		}
		target="$(cli apps deploys list "$a" --json | jq -r '[.[] | select(.status == "succeeded")][1].id // ""')"
		[ -n "$target" ] || return 1
		cli apps deploys rollback-to "$a" "$target" --confirm >/dev/null || return 1
	done
	for a in "${apps[@]}"; do
		poll body_has "$(domain_of "$a")" v1 || {
			log "$a did not return to marker v1 after rollback"
			return 1
		}
	done
}

start_load() {
	local a d
	rm -f "$work/stop"
	for a in "${apps[@]}"; do
		d="$(domain_of "$a")"
		: >"$work/load-$a.codes"
		(
			while [ ! -e "$work/stop" ]; do
				code="$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 "https://$d/healthz" || true)"
				echo "${code:-000}" >>"$work/load-$a.codes"
				sleep 0.1
			done
		) &
		load_pids+=("$!")
	done
}

step_upgrade() {
	local a total bad sum_total=0 sum_bad=0 rc=0
	start_load
	sleep 3
	remote_root env LEVELRAIL_VERSION="$SMOKE_TO_TAG" LEVELRAIL_SKIP_REACHABILITY=1 sh -s upgrade <"$repo_root/install.sh" >"$work/upgrade.log" 2>&1 || {
		tail -n 30 "$work/upgrade.log" >&2
		rc=1
	}
	poll curl -fsS --max-time 5 -o /dev/null "$api/api/v1/auth/setup-status" || rc=1
	sleep 10
	stop_load
	for a in "${apps[@]}"; do
		total="$(wc -l <"$work/load-$a.codes" | tr -d ' ')"
		bad="$(grep -vc '^2' "$work/load-$a.codes" || true)"
		log "$a: requests=$total non2xx=$bad"
		sum_total=$((sum_total + total))
		sum_bad=$((sum_bad + bad))
	done
	UPGRADE_DETAIL="requests=${sum_total} non2xx=${sum_bad}"
	[ "$sum_total" -gt 0 ] && [ "$sum_bad" -eq 0 ] && [ "$rc" -eq 0 ]
}
UPGRADE_DETAIL=""

STEPS=(
	"prereqs|check local tools, build the CLI, test ssh access"
	"install|run install.sh at SMOKE_FROM_TAG on the host over ssh, wait for the API"
	"admin|read the one-time setup token, register the admin, mint an API token"
	"seed_repo|push the three fixtures to a scratch branch of SMOKE_REPO (marker v1)"
	"connect_github|create the three apps and attach the git source with SMOKE_GH_TOKEN"
	"deploy_fixtures|add domains, trigger builds, wait for deploys, assert body marker and /healthz"
	"tls|assert each domain serves a verified chain, hostname match, and 7+ days to notAfter"
	"rollback|push marker v2, redeploy, roll back to the previous succeeded deploy, assert marker v1"
	"upgrade|upgrade to SMOKE_TO_TAG under continuous curl load, assert zero non-2xx"
)

print_plan() {
	local i=1 s
	for s in "${STEPS[@]}"; do
		printf 'step %d: %-16s %s\n' "$i" "${s%%|*}" "${s#*|}"
		i=$((i + 1))
	done
}

main() {
	if [ "$dry_run" -eq 1 ]; then
		echo "dry run, no ssh, no network. Planned steps:"
		print_plan
		return 0
	fi

	local v
	for v in SMOKE_HOST SMOKE_DOMAIN_BASE SMOKE_GH_TOKEN SMOKE_REPO SMOKE_FROM_TAG SMOKE_TO_TAG; do
		[ -n "${!v:-}" ] || {
			echo "missing required env: $v" >&2
			return 2
		}
	done

	work="$(mktemp -d -t vps-smoke.XXXXXX)"
	trap cleanup EXIT
	host_addr="${SMOKE_HOST#*@}"
	api="http://${host_addr}:8080"
	cli_bin=""

	local s name detail
	for s in "${STEPS[@]}"; do
		name="${s%%|*}"
		log "step: $name"
		if "step_$name"; then
			detail=""
			[ "$name" != upgrade ] || detail="$UPGRADE_DETAIL"
			record "$name" PASS "$detail"
		else
			detail=""
			[ "$name" != upgrade ] || detail="$UPGRADE_DETAIL"
			record "$name" FAIL "$detail"
			# Later steps depend on earlier ones, except that a failed TLS
			# assertion should not hide the rollback and upgrade results.
			case "$name" in
			tls) ;;
			*) break ;;
			esac
		fi
	done

	echo
	printf '%-34s %-5s %s\n' STEP RESULT DETAIL
	printf '%s\n' "${results[@]}"
	return "$failed"
}

main
