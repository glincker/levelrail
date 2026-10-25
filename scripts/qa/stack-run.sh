#!/usr/bin/env bash
# Runs one QA scenario stack end to end: apply, wait healthy, assert, destroy.
# Works against scripts/dev-local.sh (login dev/dev) or a droplet from droplet.sh.
#
# Usage: scripts/qa/stack-run.sh <hello-web|app-db|rollback-loop>
#          [--server URL] [--token T] [--keep] [--dry-run]
# Env (all optional):
#   QA_CLI              levelrail-cli binary (default: levelrail-cli on PATH)
#   QA_IMAGE_A/_B       image tags (default traefik/whoami v1.10.3 / v1.10.2)
#   QA_BODY_MATCH       text the response body must contain (default Hostname)
#   QA_STACK_WAIT       seconds to wait for healthy (default 300)
#   QA_LOOP_INTERVAL    seconds between loop requests (default 0.2)
#   QA_LOOP_MAX_BAD     non-200 responses tolerated during rollback (default 0)
#   QA_INGRESS_HTTP_PORT/_HTTPS_PORT  server ingress ports for the hostname check (80/443)
#   QA_APP_URL         assert against this URL instead of the app hostname
set -uo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cli="${QA_CLI:-levelrail-cli}"
image_a="${QA_IMAGE_A:-traefik/whoami:v1.10.3}"
image_b="${QA_IMAGE_B:-traefik/whoami:v1.10.2}"
body_match="${QA_BODY_MATCH:-Hostname}"
wait_secs="${QA_STACK_WAIT:-300}"
http_port="${QA_INGRESS_HTTP_PORT:-80}"
https_port="${QA_INGRESS_HTTPS_PORT:-443}"
loop_interval="${QA_LOOP_INTERVAL:-0.2}"
loop_max_bad="${QA_LOOP_MAX_BAD:-0}"
dry_run=0
keep=0
server=""

usage() {
	echo "usage: $0 <hello-web|app-db|rollback-loop> [--server URL] [--token T] [--keep] [--dry-run]" >&2
	exit 2
}

scenario_known() {
	case "$1" in hello-web | app-db | rollback-loop) return 0 ;; *) return 1 ;; esac
}

# Unique per run so parallel runs and leftovers never collide.
make_suffix() {
	printf '%s%s' "$(date +%s | tail -c 5)" "$(LC_ALL=C tr -dc 'a-z0-9' </dev/urandom | head -c 3)"
}

# The app a scenario asserts against.
app_name() {
	case "$1" in
	hello-web) echo "qa-hello-$2" ;;
	app-db) echo "qa-appdb-$2" ;;
	rollback-loop) echo "qa-roll-$2" ;;
	esac
}

# The image is filled in here: ${{ env.X }} placeholders are only resolved in
# env values by the stack engine, not in build.image.
render() { # scenario suffix outdir image
	sed -e "s/@SUFFIX@/$2/g" -e "s|@IMAGE@|$4|g" "$here/stacks/$1.yaml" >"$3/$1.yaml"
}

run() {
	if [ "$dry_run" = 1 ]; then
		echo "+ $*"
		return 0
	fi
	"$@"
}

lr() { run "$cli" "$@"; }

app_host() {
	"$cli" open "$1" --json 2>/dev/null | grep -o '[A-Za-z0-9.-]*\.sslip\.io' | head -1
}

# Prints "body\ncode" for the app, resolving its zero-config hostname to the server.
fetch() {
	local app="$1" host ip
	if [ -n "${QA_APP_URL:-}" ]; then
		curl -sk --max-time 3 -w '\n%{http_code}' "$QA_APP_URL"
		return
	fi
	host="$(app_host "$app")"
	[ -n "$host" ] || return 1
	ip="$(printf '%s' "$server" | sed -E 's#^[a-z]+://##; s#[:/].*##')"
	curl -skL --max-time 3 --resolve "$host:$http_port:$ip" --resolve "$host:$https_port:$ip" -w '\n%{http_code}' "http://$host:$http_port/"
}

wait_healthy() {
	if [ "$dry_run" = 1 ]; then
		lr apps deploys wait "$1" --timeout "${wait_secs}s"
		return
	fi
	"$cli" apps deploys wait "$1" --timeout "${wait_secs}s" >/dev/null 2>&1
}

assert_http() {
	local out code="" body deadline=$((SECONDS + wait_secs))
	while [ $SECONDS -lt $deadline ]; do
		out="$(fetch "$1" 2>/dev/null)" || out=""
		code="${out##*$'\n'}"
		body="${out%$'\n'*}"
		if [ "$code" = 200 ] && [[ "$body" == *"$body_match"* ]]; then return 0; fi
		sleep 2
	done
	echo "last response code=${code:-none}" >&2
	return 1
}

# A first apply needs its project to exist, so each run makes its own.
project_name() { echo "qa-$1"; }

create_project() {
	lr apps projects create --name "$(project_name "$1")" >/dev/null
}

delete_project() {
	local id
	id="$("$cli" apps projects list --json 2>/dev/null | tr -d ' \n' | grep -o "\"id\":\"[^\"]*\",\"name\":\"$(project_name "$1")\"" | head -1 | sed -E 's/"id":"([^"]*)".*/\1/')"
	[ -z "$id" ] || "$cli" apps projects delete "$id" >/dev/null 2>&1
}

apply_stack() { # dir name image suffix scenario
	render "$5" "$4" "$1" "$3"
	lr stack apply "$2" -f "$1" --project "$(project_name "$4")" --yes
}

# Second tag, then rollback to revision 1 while a request loop runs.
scenario_rollback() { # app stack dir suffix scenario
	local app="$1" stack="$2" dir="$3" suffix="$4" scenario="$5" log bad total pid rc
	apply_stack "$dir" "$stack" "$image_b" "$suffix" "$scenario" || return 1
	wait_healthy "$app" || return 1
	if [ "$dry_run" = 1 ]; then
		echo "+ (200ms request loop against $app while: $cli stack rollback $stack 1 --yes)"
		return 0
	fi
	log="$(mktemp)"
	(while :; do
		printf '%s\n' "$(fetch "$app" 2>/dev/null | tail -n1)" >>"$log"
		sleep "$loop_interval"
	done) &
	pid=$!
	"$cli" stack rollback "$stack" 1 --yes
	rc=$?
	sleep 3
	kill "$pid" 2>/dev/null
	wait "$pid" 2>/dev/null
	total="$(wc -l <"$log" | tr -d ' ')"
	bad="$(grep -vc '^200$' "$log" || true)"
	rm -f "$log"
	echo "rollback loop: $total requests, $bad non-200" >&2
	[ "$rc" = 0 ] && [ "$bad" -le "$loop_max_bad" ] && [ "$total" -gt 0 ]
}

main() {
	local scenario="${1:-}" suffix stack dir app status=PASS reason=""
	[ -n "$scenario" ] || usage
	shift
	scenario_known "$scenario" || usage
	server="${APP_API_URL:-http://127.0.0.1:8080}"
	local token="${APP_API_TOKEN:-}"
	while [ $# -gt 0 ]; do
		case "$1" in
		--server) server="${2:?}"; shift 2 ;;
		--token) token="${2:?}"; shift 2 ;;
		--keep) keep=1; shift ;;
		--dry-run) dry_run=1; shift ;;
		*) usage ;;
		esac
	done
	export APP_API_URL="$server"
	[ -z "$token" ] || export APP_API_TOKEN="$token"
	export APP_EXPERIMENTAL="${APP_EXPERIMENTAL:-iac}"

	suffix="$(make_suffix)"
	stack="qa-$scenario-$suffix"
	app="$(app_name "$scenario" "$suffix")"
	dir="$(mktemp -d)"

	create_project "$suffix" || { status=FAIL; reason="create project"; }
	if [ "$status" = PASS ]; then
		apply_stack "$dir" "$stack" "$image_a" "$suffix" "$scenario" || { status=FAIL; reason="apply"; }
	fi
	if [ "$status" = PASS ]; then
		wait_healthy "$app" || { status=FAIL; reason="not healthy"; }
	fi
	if [ "$status" = PASS ] && [ "$scenario" = rollback-loop ]; then
		scenario_rollback "$app" "$stack" "$dir" "$suffix" "$scenario" || { status=FAIL; reason="rollback loop"; }
	elif [ "$status" = PASS ]; then
		if [ "$dry_run" = 1 ]; then
			echo "+ (poll app hostname until 200 and body contains '$body_match', up to ${wait_secs}s)"
		else
			assert_http "$app" || { status=FAIL; reason="http assertion"; }
		fi
	fi

	if [ "$keep" = 1 ]; then
		echo "keeping stack $stack" >&2
	elif [ "$dry_run" = 1 ]; then
		echo "+ $cli stack delete $stack --mode destroy --yes"
	else
		"$cli" stack delete "$stack" --mode destroy --yes >/dev/null 2>&1 || echo "destroy failed for $stack" >&2
		# Destroy only forgets a stack's databases, so remove the scenario's own.
		[ "$scenario" != app-db ] || "$cli" databases delete "qa-db-$suffix" --delete-data --confirm "qa-db-$suffix" >/dev/null 2>&1
		delete_project "$suffix"
	fi
	find "$dir" -mindepth 1 -delete
	rmdir "$dir"
	echo "$status $scenario $stack${reason:+ ($reason)}"
	[ "$status" = PASS ]
}

if [ "${BASH_SOURCE[0]}" = "$0" ]; then
	main "$@"
fi
