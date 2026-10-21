#!/usr/bin/env bash
# First-hour smoke test: what a new user does, from install to rollback, in a
# systemd Ubuntu container on the local Docker. See docs/ci.md.
#   scripts/test-first-hour.sh [--keep] [--steps 4-7] [--container NAME]
#                              [--binary PATH] [--cli PATH] [--image BASE]
# Without --binary/--cli the linux binaries are built with the local Go toolchain.
# Steps 2-10 need the state of earlier steps: a subset only works against a
# container kept by a previous --keep run, passed back with --container.
set -u

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
base_image="${FIRST_HOUR_BASE_IMAGE:-ubuntu:24.04}"
app_image="${FIRST_HOUR_APP_IMAGE:-traefik/whoami:v1.10.3}"
total_timeout="${FIRST_HOUR_TIMEOUT:-1500}"
install_wait="${FIRST_HOUR_INSTALL_WAIT:-900}"
boot_wait="${FIRST_HOUR_BOOT_WAIT:-90}"
ready_wait="${FIRST_HOUR_READY_WAIT:-120}"
cutover_wait="${FIRST_HOUR_CUTOVER_WAIT:-180}"
doctor_wait="${FIRST_HOUR_DOCTOR_WAIT:-120}"
diagnose_wait="${FIRST_HOUR_DIAGNOSE_WAIT:-90}"
port_wait="${FIRST_HOUR_PORT_WAIT:-30}"
app_name="${FIRST_HOUR_APP_NAME:-smoke}"
admin_user="admin@first-hour.test"
admin_pass="first-hour-Pass-1234"
made_up_domain="first-hour.lr.test"

keep=0
steps_arg=""
name=""
binary=""
cli_binary=""
reuse=0
while [ $# -gt 0 ]; do
	case "$1" in
	--keep) keep=1 ;;
	--steps) steps_arg="${2:?--steps needs N or N-M}"; shift ;;
	--container) name="${2:?--container needs a name}"; reuse=1; keep=1; shift ;;
	--binary) binary="${2:?--binary needs a path}"; shift ;;
	--cli) cli_binary="${2:?--cli needs a path}"; shift ;;
	--image) base_image="${2:?--image needs an image}"; shift ;;
	-h | --help) sed -n '2,8p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
	*) echo "unknown argument: $1" >&2; exit 2 ;;
	esac
	shift
done

[ -n "$name" ] || name="first-hour-$$"
tag="first-hour-test:$(printf '%s' "$base_image" | tr ':/' '--')"
work="$(mktemp -d "${TMPDIR:-/tmp}/first-hour.XXXXXX")"
watchdog_pid=""
results=""
failures=0
FAIL_DETAIL=""
app_v1="first-hour-app:v1"
app_v2="first-hour-app:v2"

STEP_TABLE='step_install|installer completes and the control plane is healthy
step_setup_link|setup link works and an admin is created through the API
step_cli_login|levelrail-cli signs in
step_deploy|sample app deploys and becomes ready
step_hostname|app answers on its zero-config hostname through the ingress
step_cutover|second deploy cuts over with zero failed requests
step_rollback|rollback returns the previous version
step_doctor|doctor reports no failures
step_diagnose|domain diagnose on a made-up hostname returns a structured report
step_uninstall|uninstall leaves no listening ports'

now() { date +%s; }

cleanup() {
	[ -z "$watchdog_pid" ] || kill "$watchdog_pid" >/dev/null 2>&1 || true
	if [ "$keep" -eq 1 ]; then
		echo "kept container $name (docker exec -it $name bash), work dir $work"
	else
		docker rm -fv "$name" >/dev/null 2>&1 || true
		docker rmi "$tag" "$app_v1" "$app_v2" >/dev/null 2>&1 || true
		rm -rf "$work"
	fi
}
trap cleanup EXIT
trap 'echo "FAIL overall timeout of ${total_timeout}s reached"; summary; exit 124' TERM
trap 'echo "interrupted"; exit 130' INT

in_ct() { docker exec "$name" sh -c "$1"; }
lr() { docker exec -e HOME=/root "$name" levelrail-cli "$@"; }

# poll DESC TIMEOUT CMD...: bounded wait, prints what it waits for and how long it took.
poll() {
	local desc="$1" limit="$2" start waited
	shift 2
	start="$(now)"
	echo "    waiting for ${desc} (up to ${limit}s)"
	while ! "$@" >/dev/null 2>&1; do
		waited=$(($(now) - start))
		if [ "$waited" -ge "$limit" ]; then
			echo "    gave up after ${waited}s waiting for ${desc}"
			return 1
		fi
		sleep 1
	done
	echo "    ${desc} after $(($(now) - start))s"
}

fail() {
	FAIL_DETAIL="$1"
	return 1
}

summary() {
	echo
	echo "== summary"
	printf '%s' "$results"
	if [ "$failures" -eq 0 ]; then
		echo "first-hour smoke test passed"
	else
		echo "first-hour smoke test FAILED: $failures step(s)"
	fi
}

in_range() {
	local n="$1" lo hi
	[ -n "$steps_arg" ] || return 0
	case "$steps_arg" in
	*-*) lo="${steps_arg%-*}"; hi="${steps_arg#*-}" ;;
	*) lo="$steps_arg"; hi="$steps_arg" ;;
	esac
	[ "$n" -ge "$lo" ] && [ "$n" -le "$hi" ]
}

# host_arch maps the Docker daemon's architecture to GOARCH.
host_arch() {
	case "$(docker version --format '{{.Server.Arch}}')" in
	arm64 | aarch64) echo arm64 ;;
	*) echo amd64 ;;
	esac
}

prepare_binaries() {
	local arch
	arch="$(host_arch)"
	if [ -z "$binary" ]; then
		echo "== building linux/$arch control plane"
		binary="$work/levelrail"
		(cd "$repo_root" && CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -o "$binary" ./cmd/levelrail) || return 1
	fi
	if [ -z "$cli_binary" ]; then
		echo "== building linux/$arch CLI"
		cli_binary="$work/levelrail-cli"
		(cd "$repo_root" && CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -o "$cli_binary" ./cmd/levelrail-cli) || return 1
	fi
}

# prepare_app_images builds two distinguishable versions of the sample app on the host, so the
# container needs no registry access.
prepare_app_images() {
	local v
	docker image inspect "$app_image" >/dev/null 2>&1 || docker pull -q "$app_image" >/dev/null || return 1
	for v in 1 2; do
		docker build -q -t "first-hour-app:v$v" - >/dev/null <<EOF || return 1
FROM ${app_image}
ENV WHOAMI_NAME=first-hour-v${v}
EOF
	done
	docker save -o "$work/apps.tar" "$app_v1" "$app_v2"
}

boot_container() {
	docker build -q -t "$tag" - >/dev/null <<EOF || return 1
FROM ${base_image}
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update -qq && apt-get install -y -qq --no-install-recommends \
      systemd systemd-sysv dbus curl ca-certificates iproute2 procps python3 >/dev/null \
 && rm -rf /var/lib/apt/lists/* \
 && systemctl mask getty.target console-getty.service systemd-logind.service
STOPSIGNAL SIGRTMIN+3
CMD ["/sbin/init"]
EOF
	docker run -d --name "$name" --privileged --cgroupns=host \
		-v /sys/fs/cgroup:/sys/fs/cgroup:rw --tmpfs /run --tmpfs /run/lock \
		-v /var/lib/docker -v /var/lib/containerd "$tag" >/dev/null || return 1
	poll "systemd to boot" "$boot_wait" in_ct 'systemctl is-system-running 2>/dev/null | grep -Eq "running|degraded"' || return 1
	docker cp "$repo_root/install.sh" "$name:/root/install.sh"
	docker cp "$binary" "$name:/root/levelrail"
	docker cp "$cli_binary" "$name:/usr/local/bin/levelrail-cli"
	in_ct 'chmod 755 /usr/local/bin/levelrail-cli'
}

# app_get_name prints the Name line the sample app answers with, via the ingress.
app_get_name() {
	in_ct "sh /root/fh/get.sh" 2>&1 | sed -n 's/^Name: //p' | head -1
}

app_is() {
	[ "$(app_get_name)" = "$1" ]
}

step_install() {
	local out
	out="$(docker exec "$name" timeout -k 10 "$install_wait" env \
		LEVELRAIL_BINARY_FILE=/root/levelrail LEVELRAIL_PUBLIC_IP=127.0.0.1 LEVELRAIL_MIN_DISK_GB=1 \
		sh /root/install.sh 2>&1)"
	local rc=$?
	in_ct 'mkdir -p /root/fh'
	printf '%s\n' "$out" | docker exec -i "$name" sh -c 'cat > /root/fh/install.out'
	[ "$rc" -eq 0 ] || fail "installer exited $rc: $(printf '%s' "$out" | tail -25)" || return 1
	poll "/healthz" 60 in_ct 'curl -fsS --max-time 3 http://127.0.0.1:8080/healthz' || fail "control plane not healthy: $(in_ct 'journalctl -u levelrail -n 30 --no-pager' 2>&1)" || return 1
	in_ct 'systemctl is-active --quiet levelrail' || fail "levelrail unit not active" || return 1
}

step_setup_link() {
	local link token status resp page
	link="$(in_ct 'cat /root/fh/install.out' | grep -o 'http://[^ ]*/login#setup=[A-Za-z0-9_-]*' | head -1)"
	[ -n "$link" ] || fail "no /login#setup=<token> link in installer output: $(in_ct 'tail -25 /root/fh/install.out')" || return 1
	token="${link#*#setup=}"
	status="$(in_ct 'curl -fsS --max-time 5 http://127.0.0.1:8080/api/v1/auth/setup-status')"
	case "$status" in *'"needs_setup":true'*) ;; *) fail "setup-status before register: $status" || return 1 ;; esac
	page="$(in_ct "curl -sS --max-time 5 -w '\n%{http_code}' http://127.0.0.1:8080/login" 2>&1)"
	case "$page" in
	*"frontend not embedded"*) echo "    login page not checked: this binary was built without the embedded dashboard" ;;
	*$'\n'200) ;;
	*) fail "GET /login did not return 200: $page" || return 1 ;;
	esac
	resp="$(in_ct "curl -sS --max-time 10 -c /root/fh/jar -w '\n%{http_code}' -H 'Content-Type: application/json' \
		-d '{\"username\":\"$admin_user\",\"password\":\"$admin_pass\",\"setup_token\":\"$token\"}' \
		http://127.0.0.1:8080/api/v1/auth/register" 2>&1)"
	case "$resp" in *$'\n'20[01]) ;; *) fail "register with the link's token failed: $resp" || return 1 ;; esac
	status="$(in_ct 'curl -fsS --max-time 5 http://127.0.0.1:8080/api/v1/auth/setup-status')"
	case "$status" in *'"needs_setup":false'*) ;; *) fail "setup-status after register: $status" || return 1 ;; esac
}

# The dashboard's "Connect your CLI" card issues a pairing code to the signed-in session (the admin
# cookie saved at setup plays that session); the CLI signs in with it. A password-only sign-in from
# a new device must still be blocked while that session is live.
step_cli_login() {
	local out code
	out="$(in_ct "curl -sS --max-time 10 -b /root/fh/jar -w '\n%{http_code}' -X POST -H 'Content-Type: application/json' -d '{}' http://127.0.0.1:8080/api/v1/auth/cli-pairing" 2>&1)"
	case "$out" in *$'\n'201) ;; *) fail "issuing a CLI pairing code from the admin session failed: $out" || return 1 ;; esac
	code="$(printf '%s' "$out" | sed -n 's/.*"code":"\([A-Z2-7]\{4\}-[A-Z2-7]\{4\}\)".*/\1/p' | head -1)"
	[ -n "$code" ] || fail "no pairing code in the response" || return 1
	out="$(docker exec -e HOME=/root "$name" levelrail-cli login --api-url http://127.0.0.1:8080 --code "$code" 2>&1)" ||
		fail "signing in with the pairing code failed: $out" || return 1
	case "$out" in *"logged in as"*) ;; *) fail "pairing sign-in did not report success: $out" || return 1 ;; esac
	out="$(lr apps list --api-url http://127.0.0.1:8080 2>&1)" || fail "apps list with the paired token failed: $out" || return 1
	if docker exec -e HOME=/root "$name" levelrail-cli login --api-url http://127.0.0.1:8080 --code "$code" >/root/fh/reuse.out 2>&1; then
		fail "a pairing code worked twice" || return 1
	fi
	if out="$(docker exec -e HOME=/tmp/fh-blocked "$name" levelrail-cli auth login --api-url http://127.0.0.1:8080 --username "$admin_user" --password "$admin_pass" 2>&1)"; then
		fail "a password-only sign-in from a new device was not blocked: $out" || return 1
	fi
	case "$out" in *"approval required"*) ;; *) fail "password sign-in failed for the wrong reason, expected the new-device approval error: $out" || return 1 ;; esac
}

step_deploy() {
	local out
	docker exec -i "$name" docker load <"$work/apps.tar" >/dev/null 2>&1 || fail "could not load the sample app images into the container Docker" || return 1
	out="$(lr apps create --name "$app_name" --image "$app_v1" --port 80 2>&1)" || fail "apps create: $out" || return 1
	out="$(lr apps deploy "$app_name" --image "$app_v1" 2>&1)" || fail "apps deploy: $out" || return 1
	poll "$app_name to be running" "$ready_wait" in_ct "docker ps --format '{{.Names}} {{.Image}}' | grep -q '$app_v1'" ||
		fail "no container for $app_v1: $(in_ct 'docker ps -a' 2>&1) $(lr apps status "$app_name" 2>&1)" || return 1
	lr apps deploys wait "$app_name" --timeout "${ready_wait}s" >"$work/wait.out" 2>&1 ||
		fail "deploy did not become healthy: $(cat "$work/wait.out") $(lr apps status "$app_name" 2>&1)" || return 1
}

step_hostname() {
	local out host
	out="$(lr open "$app_name" --json 2>&1)" || fail "open: $out" || return 1
	host="$(printf '%s' "$out" | grep -o '[A-Za-z0-9.-]*\.sslip\.io' | head -1)"
	[ -n "$host" ] || fail "no zero-config sslip.io hostname in: $out" || return 1
	printf '%s' "$host" >"$work/host"
	printf 'curl -sS -k -L --max-time 3 --resolve %s:80:127.0.0.1 --resolve %s:443:127.0.0.1 "$@" http://%s/\n' "$host" "$host" "$host" >"$work/get.sh"
	docker cp "$work/get.sh" "$name:/root/fh/get.sh"
	# shellcheck disable=SC2016 # the loop script expands these inside the container
	printf '%s\n' 'while [ ! -e /tmp/loop.stop ]; do' \
		'  code=$(sh /root/fh/get.sh -o /tmp/loop.body -w "%{http_code}" 2>/dev/null)' \
		'  echo "$code $(sed -n "s/^Name: //p" /tmp/loop.body | head -1)" >> /tmp/loop.log' \
		'  sleep 0.1' 'done' >"$work/loop.sh"
	docker cp "$work/loop.sh" "$name:/root/fh/loop.sh"
	poll "$host to answer through the ingress" "$ready_wait" app_is first-hour-v1 ||
		fail "ingress did not serve first-hour-v1 on $host; got: $(in_ct "curl -sS -k -i --max-time 5 --resolve '$host:80:127.0.0.1' --resolve '$host:443:127.0.0.1' http://$host/ 2>&1 | head -20")" || return 1
}

step_cutover() {
	local out bad total
	in_ct 'rm -f /tmp/loop.log /tmp/loop.stop; nohup sh /root/fh/loop.sh >/dev/null 2>&1 &'
	poll "the request loop to start" 15 in_ct 'test -s /tmp/loop.log' || fail "request loop did not start" || return 1
	out="$(lr apps deploy "$app_name" --image "$app_v2" 2>&1)" || { in_ct 'touch /tmp/loop.stop'; fail "second deploy: $out" || return 1; }
	poll "traffic to move to first-hour-v2" "$cutover_wait" app_is first-hour-v2 || { in_ct 'touch /tmp/loop.stop'; fail "never saw first-hour-v2; $(lr apps status "$app_name" 2>&1)" || return 1; }
	lr apps deploys wait "$app_name" --timeout "${cutover_wait}s" >"$work/wait.out" 2>&1 || true
	sleep 3
	in_ct 'touch /tmp/loop.stop'
	sleep 1
	total="$(in_ct 'wc -l < /tmp/loop.log' | tr -d ' ')"
	bad="$(in_ct "grep -v '^200 first-hour-v[12]\$' /tmp/loop.log | sort | uniq -c | head -20")"
	echo "    $total requests during the cutover"
	[ -z "$bad" ] || fail "failed requests during the cutover ($total total): $bad" || return 1
	in_ct 'grep -q "first-hour-v1" /tmp/loop.log && grep -q "first-hour-v2" /tmp/loop.log' || fail "loop did not observe both versions: $(in_ct 'sort /tmp/loop.log | uniq -c')" || return 1
}

step_rollback() {
	local out
	out="$(lr apps rollback "$app_name" 2>&1)" || fail "rollback: $out" || return 1
	printf '%s\n' "$out" | sed 's/^/    rollback: /' | head -5
	poll "the previous version to answer" "$cutover_wait" app_is first-hour-v1 ||
		fail "old response did not return; rollback said: $out; status: $(lr apps status "$app_name" 2>&1)" || return 1
	out="$(lr apps get "$app_name" --json 2>&1)"
	case "$out" in *'"image": "'"$app_v1"'@sha256:'*) ;; *) fail "app record does not point at $app_v1 after rollback: $out" || return 1 ;; esac
	sleep 2
	app_is first-hour-v1 || fail "the old version did not keep answering 2s after rollback, got: $(app_get_name)" || return 1
}

step_doctor() {
	local out rc=0
	out="$(docker exec -e HOME=/root "$name" timeout -k 10 "$doctor_wait" levelrail-cli doctor 2>&1)" || rc=$?
	printf '%s\n' "$out" | sed 's/^/    doctor: /' | head -40
	[ "$rc" -eq 0 ] || fail "doctor exited $rc: $out" || return 1
	if printf '%s' "$out" | grep -Eiq '(^|[[:space:]|])fail([[:space:]|]|$)'; then
		fail "doctor reported a failing check: $out" || return 1
	fi
}

step_diagnose() {
	local out keys rc=0
	out="$(lr apps domains add "$app_name" "$made_up_domain" 2>&1)" || fail "domains add: $out" || return 1
	out="$(docker exec -e HOME=/root "$name" timeout -k 10 "$diagnose_wait" levelrail-cli domains diagnose "$made_up_domain" --json 2>&1)" || rc=$?
	[ "$rc" -ne 124 ] && [ "$rc" -ne 137 ] || fail "domains diagnose hung past ${diagnose_wait}s" || return 1
	keys="$(printf '%s' "$out" | docker exec -i "$name" python3 -c 'import json,sys; d=json.load(sys.stdin); assert isinstance(d,(dict,list)) and d; print(" ".join(sorted(d)) if isinstance(d,dict) else len(d))' 2>/dev/null)" ||
		fail "domains diagnose (exit $rc) did not print a JSON report: $out" || return 1
	echo "    diagnose report fields: $keys (exit $rc)"
	lr apps domains remove "$app_name" "$made_up_domain" >/dev/null 2>&1 || true
}

# listeners prints every TCP listener except the resolver stub and the loopback ports Docker
# publishes for app containers, which uninstall documents as left running.
listeners() {
	in_ct "ss -ltnpH | grep -v docker-proxy | awk '{print \$4}' | grep -v '^127.0.0.53'"
}

no_listeners() { ! listeners | grep -q .; }

step_uninstall() {
	local out
	out="$(in_ct 'sh /root/install.sh uninstall --purge' 2>&1)" || fail "uninstall: $out" || return 1
	if in_ct 'systemctl is-active --quiet levelrail'; then fail "service still active after uninstall" || return 1; fi
	poll "all listeners to close" "$port_wait" no_listeners ||
		fail "ports still listening after uninstall: $(listeners)" || return 1
}

run_steps() {
	local n=0 fn title start dur status
	while IFS='|' read -r fn title; do
		n=$((n + 1))
		in_range "$n" || continue
		echo "== step $n: $title"
		start="$(now)"
		FAIL_DETAIL=""
		if "$fn"; then
			status="PASS"
		else
			status="FAIL"
		fi
		dur=$(($(now) - start))
		if [ "$status" = "FAIL" ]; then
			failures=$((failures + 1))
			echo "FAIL step $n: $title (${dur}s)"
			printf '%s\n' "$FAIL_DETAIL" | sed 's/^/    | /'
		else
			echo "PASS step $n: $title (${dur}s)"
		fi
		results="${results}$(printf '%-6s step %2d  %4ss  %s' "$status" "$n" "$dur" "$title")
"
		[ "$status" != "FAIL" ] || [ "$n" -ge 10 ] || { echo "stopping: later steps depend on this one"; break; }
	done <<EOF
$STEP_TABLE
EOF
}

df_free_gb() { df -Pk / | awk 'NR==2 {printf "%d", $4/1048576}'; }
[ "$(df_free_gb)" -ge 5 ] || { echo "less than 5G free on /, refusing to start"; exit 2; }

( sleep "$total_timeout"; kill -TERM $$ ) >/dev/null 2>&1 &
watchdog_pid=$!

if [ "$reuse" -eq 0 ]; then
	prepare_binaries || { echo "could not build binaries"; exit 1; }
	prepare_app_images || { echo "could not prepare the sample app images"; exit 1; }
	echo "== container ($base_image)"
	boot_container || { echo "could not boot the systemd container"; exit 1; }
else
	docker inspect "$name" >/dev/null 2>&1 || { echo "container $name not found"; exit 1; }
	prepare_app_images || { echo "could not prepare the sample app images"; exit 1; }
fi

run_steps
summary
[ "$failures" -eq 0 ]
