#!/usr/bin/env bash
# Tests for droplet.sh helpers and dry-run output. doctl, ssh and rsync are
# stubs on PATH that fail loudly, so a test can never touch a real droplet.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
tmp="$(mktemp -d)"
trap 'find "$tmp" -mindepth 1 -delete; rmdir "$tmp"' EXIT

mkdir "$tmp/bin"
for tool in doctl ssh rsync; do
	printf '#!/usr/bin/env bash\necho "stub %s called: $*" >&2\nexit 99\n' "$tool" >"$tmp/bin/$tool"
	chmod +x "$tmp/bin/$tool"
done
export PATH="$tmp/bin:$PATH"
export QA_CACHE="$tmp/cache"

pass=0
fail=0
check() {
	local name="$1" got="$2" want="$3"
	if [[ "$got" == "$want" ]]; then
		pass=$((pass + 1))
	else
		fail=$((fail + 1))
		printf 'FAIL %s\n  got:  %s\n  want: %s\n' "$name" "$got" "$want"
	fi
}
contains() {
	local name="$1" hay="$2" needle="$3"
	if [[ "$hay" == *"$needle"* ]]; then
		pass=$((pass + 1))
	else
		fail=$((fail + 1))
		printf 'FAIL %s: missing %s in:\n%s\n' "$name" "$needle" "$hay"
	fi
}

# shellcheck source=droplet.sh
source "$here/droplet.sh"

check runtime_docker "$(valid_runtime docker && echo y)" y
check runtime_bad "$(valid_runtime lxc && echo y || echo n)" n
check arch_x86 "$(arch_of_uname x86_64)" amd64
check arch_arm "$(arch_of_uname aarch64)" arm64
check arch_bad "$(arch_of_uname riscv64 && echo y || echo n)" n
check sum_hourly "$(printf 'a 0.00893\nb 0.01786\n' | sum_hourly)" "2 0.02679"
check monthly "$(monthly_from_hourly 0.01)" "7.30"

k1="$(build_key abc clean amd64)"
check key_stable "$(build_key abc clean amd64)" "$k1"
check key_len "${#k1}" 16
check key_arch_differs "$([[ "$(build_key abc clean arm64)" != "$k1" ]] && echo y)" y
check key_dirty_differs "$([[ "$(build_key abc deadbeef amd64)" != "$k1" ]] && echo y)" y

printf 'hello' >"$tmp/f"
check sha256 "$(sha256_file "$tmp/f")" "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"

n=0
flaky() {
	n=$((n + 1))
	[[ "$n" -ge 3 ]]
}
sleep() { :; }
check retry_ok "$(retry 5 flaky 2>/dev/null && echo y)" y
n=-10
check retry_gives_up "$(retry 2 flaky 2>/dev/null && echo y || echo n)" n
unset -f sleep

# Dry runs print commands and never reach the stubs (they exit 99).
out="$("$here/droplet.sh" --dry-run up 2 --region sfo3 --size s-1vcpu-2gb --runtime podman 2>&1)"
contains up_create "$out" "doctl compute droplet create"
contains up_tag "$out" "--tag-name levelrail-qa"
contains up_region "$out" "--region sfo3"
contains up_podman "$out" "podman"

out="$("$here/droplet.sh" --dry-run down 2>&1 || true)"
contains down_needs_yes "$out" "re-run with --yes"
if [[ "$out" == *"droplet delete"* ]]; then fail=$((fail + 1)); echo "FAIL down ran delete without --yes"; fi
out="$("$here/droplet.sh" --dry-run down --yes 2>&1)"
contains down_tag_scoped "$out" "droplet delete --tag-name levelrail-qa --force"

out="$("$here/droplet.sh" --dry-run cost 2>&1)"
contains cost "$out" "1 droplets, 0.00893 USD per hour"

out="$("$here/droplet.sh" --dry-run push 192.0.2.1 2>&1)"
contains push_rsync "$out" "rsync --partial --append-verify"
contains push_atomic "$out" "/usr/local/bin/.levelrail.new"

out="$(cd "$here/../.." && "$here/droplet.sh" --dry-run build 2>&1)"
contains build_tags "$out" "-tags embedweb"
contains build_cli_no_tags "$out" "levelrail-cli"

out="$("$here/droplet.sh" bogus 2>&1 || true)"
contains unknown_cmd "$out" "unknown command"

printf '%s passed, %s failed\n' "$pass" "$fail"
[[ "$fail" == 0 ]]
