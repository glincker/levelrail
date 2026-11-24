# shellcheck shell=bash
# Helpers for real-host proofs. Sourced by run.sh and items only.

PROOF_PASSES=0
PROOF_FAILS=0
PROOF_DRY=0

proof_die() {
	printf 'host-proof: %s\n' "$1" >&2
	exit 2
}

# Refuses to run unless PROOF_HOST is an ssh target and PROOF_CONFIRM=yes.
proof_gate() {
	local host="${PROOF_HOST:-}"
	if ! printf '%s' "$host" | grep -Eq '^[A-Za-z0-9][A-Za-z0-9._@:-]*$'; then
		proof_die "set PROOF_HOST to an ssh target (for example user@host)"
	fi
	if [ "${PROOF_CONFIRM:-}" != "yes" ]; then
		proof_die "set PROOF_CONFIRM=yes to run proofs against PROOF_HOST"
	fi
}

# Masks token-shaped values so command lines and output never show secrets.
proof_redact() {
	printf '%s' "$1" | sed -E \
		-e 's/([Tt]oken|[Ss]ecret|[Pp]assword|[Pp]asswd|[Aa]pi[_-]?[Kk]ey)[=:][^[:space:]]*/\1=***/g' \
		-e 's/(^|[^A-Za-z0-9])(ghp|gho|ghs|github_pat|sk|dop_v1)[_-][A-Za-z0-9_-]{16,}/\1***/g'
}

# Quotes an argv for display, then redacts it.
proof_show() {
	local arg text=""
	for arg in "$@"; do
		text+="$(printf '%q' "$arg") "
	done
	proof_redact "${text% }"
}

# proof_check <label> <argv...>
# PROOF_LIVE unset: prints what would run, no verdict, nothing executed.
# PROOF_LIVE=1: runs argv, PASS on exit 0, else FAIL. Labels starting with TODO never run.
proof_check() {
	local label="$1" out rc=0 last verdict
	shift
	if [ "${PROOF_LIVE:-}" != "1" ]; then
		PROOF_DRY=$((PROOF_DRY + 1))
		printf 'DRY  %s\n     would run: %s\n' "$label" "$(proof_show "$@")"
		return 0
	fi
	case "$label" in
	TODO*)
		PROOF_FAILS=$((PROOF_FAILS + 1))
		printf 'FAIL %s\n     cmd: not run, check is not implemented yet\n     exit: n/a\n' "$label"
		return 0
		;;
	esac
	out=$("$@" 2>&1) || rc=$?
	last=$(printf '%s\n' "$out" | awk 'NF { last = $0 } END { print last }')
	last=$(proof_redact "$last" | cut -c1-160)
	if [ "$rc" -eq 0 ]; then
		PROOF_PASSES=$((PROOF_PASSES + 1))
		verdict=PASS
	else
		PROOF_FAILS=$((PROOF_FAILS + 1))
		verdict=FAIL
	fi
	printf '%s %s\n     cmd: %s\n     exit: %s\n     out: %s\n' \
		"$verdict" "$label" "$(proof_show "$@")" "$rc" "$last"
	return 0
}
