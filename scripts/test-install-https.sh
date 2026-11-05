#!/bin/sh
# Exercises install.sh's post-healthy HTTPS step and cloud rules with fake
# getent, levelrail-cli and binary. It extracts the two functions, so nothing
# is installed and no container is needed.
#   scripts/test-install-https.sh
set -eu

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/bin"

for fn in print_cloud_rules enable_https_after_healthy; do
	awk -v f="$fn" '$0 ~ "^"f"\\(\\) \\{" {p=1} p {print} p && /^}/ {exit}' "$repo_root/install.sh" >>"$work/fns.sh"
done
[ -s "$work/fns.sh" ] || { echo "FAIL: functions not found in install.sh"; exit 1; }

# fake binary records its arguments; FAKE_RC makes it fail.
cat >"$work/levelrail" <<'SH'
#!/bin/sh
printf '%s\n' "$*" >>"$FAKE_LOG"
exit "${FAKE_RC:-0}"
SH
cat >"$work/bin/getent" <<'SH'
#!/bin/sh
[ -z "${FAKE_DNS:-}" ] || printf '%s STREAM %s\n' "$FAKE_DNS" "$2"
SH
cat >"$work/bin/levelrail-cli" <<'SH'
#!/bin/sh
echo "Provider: Fake Cloud"
SH
chmod 755 "$work/levelrail" "$work/bin/getent" "$work/bin/levelrail-cli"

run() {
	: >"$work/log"
	# shellcheck disable=SC2016 # the snippet expands inside the child shell
	env -i PATH="$work/bin:/usr/bin:/bin" FAKE_LOG="$work/log" "$@" sh -c '
		DATA_DIR=/data; BIN_PATH="$WORK/levelrail"; HTTP_PORT=80; HTTPS_PORT=443
		log() { printf "%s\n" "$*"; }
		. "$WORK/fns.sh"
		print_cloud_rules
		enable_https_after_healthy
		echo "exit-ok"
	' 2>&1
}

expect() {
	printf '%s' "$2" | grep -qF -- "$3" || { echo "FAIL $1: missing '$3' in:"; printf '%s\n' "$2"; exit 1; }
}
refute() {
	if printf '%s' "$2" | grep -qF -- "$3"; then echo "FAIL $1: unexpected '$3' in:"; printf '%s\n' "$2"; exit 1; fi
}

base="WORK=$work DOMAIN_HINT=app.mydomain.dev EMAIL_HINT=ops@mydomain.dev PUBLIC_IP=203.0.113.9"

out="$(run $base REACH_STATE=ok FAKE_DNS=203.0.113.9)"
expect all-pass "$out" "Enabling HTTPS for app.mydomain.dev with a staging certificate first"
expect all-pass-log "$(cat "$work/log")" "enable-https --domain app.mydomain.dev --email ops@mydomain.dev --staging"
expect all-pass-next "$out" "switch to the trusted one"
expect all-pass-exit "$out" "exit-ok"

out="$(run $base REACH_STATE=ok FAKE_DNS=198.51.100.7)"
expect dns-wrong "$out" "resolves to 198.51.100.7, not 203.0.113.9"
expect dns-wrong "$out" "sudo APP_DATA_DIR=/data"
expect dns-wrong "$out" "Provider: Fake Cloud"
[ ! -s "$work/log" ] || { echo "FAIL dns-wrong: binary was run"; exit 1; }
expect dns-wrong-exit "$out" "exit-ok"

out="$(run $base REACH_STATE=ok)"
expect dns-missing "$out" "does not resolve"

out="$(run $base REACH_STATE=blocked FAKE_DNS=203.0.113.9)"
expect ports-blocked "$out" "did not answer from outside"
[ ! -s "$work/log" ] || { echo "FAIL ports-blocked: binary was run"; exit 1; }

out="$(run $base REACH_STATE=skipped FAKE_DNS=203.0.113.9)"
expect ports-skipped "$out" "were not verified"

out="$(run $base REACH_STATE=ok FAKE_DNS=203.0.113.9 FAKE_RC=3)"
expect cmd-fails "$out" "(exit 3)"
expect cmd-fails-exit "$out" "exit-ok"
refute cmd-fails "$out" "switch to the trusted one"

out="$(run WORK="$work" DOMAIN_HINT=app.mydomain.dev PUBLIC_IP=203.0.113.9 REACH_STATE=ok FAKE_DNS=203.0.113.9)"
refute no-email "$out" "enable-https"
[ ! -s "$work/log" ] || { echo "FAIL no-email: binary was run"; exit 1; }

echo "ok: install https step"
