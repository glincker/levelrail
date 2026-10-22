#!/bin/sh
# Runs install-cli.sh natively on macOS (no container):
#   scripts/test-install-cli-macos.sh <path-to-darwin-cli-binary>
# Covers the user install, the sudo install, the PATH hints per shell and
# checksum verification against a local http mirror.
set -eu

binary="$(cd "$(dirname "$1")" && pwd)/$(basename "$1")"
repo_root="$(cd "$(dirname "$0")/.." && pwd)"
script="$repo_root/install-cli.sh"
work="$(mktemp -d)"
server_pid=""
sudo_installed=""

cleanup() {
	[ -z "$server_pid" ] || kill "$server_pid" >/dev/null 2>&1 || true
	rm -rf "$work"
	if [ -n "$sudo_installed" ]; then sudo rm -f /usr/local/bin/levelrail-cli; fi
}
trap cleanup EXIT

fail() {
	echo "FAIL: $*" >&2
	exit 1
}

unset ZDOTDIR
home="$work/home"
mkdir -p "$home"

echo "== user install (zsh): installs to ~/.local/bin, prints the .zshrc line, edits nothing"
out="$(HOME="$home" SHELL=/bin/zsh PATH=/usr/bin:/bin LEVELRAIL_BINARY_FILE="$binary" sh "$script" 2>&1)"
[ -x "$home/.local/bin/levelrail-cli" ] || fail "binary not installed in ~/.local/bin: $out"
echo "$out" | grep -q '\.zshrc' || fail "no .zshrc hint: $out"
[ ! -e "$home/.zshrc" ] || fail "profile was edited without LEVELRAIL_MODIFY_PATH=1"
"$home/.local/bin/levelrail-cli" version --help >/dev/null 2>&1 || fail "levelrail-cli version failed"

echo "== LEVELRAIL_MODIFY_PATH=1 appends once (zsh)"
for _ in 1 2; do
	HOME="$home" SHELL=/bin/zsh PATH=/usr/bin:/bin LEVELRAIL_MODIFY_PATH=1 LEVELRAIL_BINARY_FILE="$binary" sh "$script" >/dev/null 2>&1
done
[ "$(grep -c 'local/bin' "$home/.zshrc")" = 1 ] || fail ".zshrc should hold the PATH line exactly once"

echo "== bash on macOS uses .bash_profile, fish uses config.fish"
HOME="$home" SHELL=/bin/bash PATH=/usr/bin:/bin LEVELRAIL_MODIFY_PATH=1 LEVELRAIL_BINARY_FILE="$binary" sh "$script" >/dev/null 2>&1
grep -q 'local/bin' "$home/.bash_profile" || fail "bash line missing from .bash_profile"
HOME="$home" SHELL=/usr/local/bin/fish PATH=/usr/bin:/bin LEVELRAIL_MODIFY_PATH=1 LEVELRAIL_BINARY_FILE="$binary" sh "$script" >/dev/null 2>&1
grep -q 'fish_add_path' "$home/.config/fish/config.fish" || fail "fish line missing from config.fish"

echo "== no hint when the dir is already on PATH"
out="$(HOME="$home" SHELL=/bin/zsh PATH="$home/.local/bin:/usr/bin:/bin" LEVELRAIL_BINARY_FILE="$binary" sh "$script" 2>&1)"
if echo "$out" | grep -q 'not on your PATH'; then fail "hint printed although dir is on PATH"; fi

echo "== checksum verification against a local mirror"
asset="levelrail-cli-darwin-$(uname -m | sed 's/x86_64/amd64/')"
mkdir -p "$work/rel/v9.9.9/good" "$work/rel/v9.9.9/tampered"
for d in good tampered; do cp "$binary" "$work/rel/v9.9.9/$d/$asset"; done
(cd "$work/rel/v9.9.9/good" && shasum -a 256 "$asset" >checksums.txt)
printf '%064d  %s\n' 0 "$asset" >"$work/rel/v9.9.9/tampered/checksums.txt"
python3 -m http.server 8099 --bind 127.0.0.1 --directory "$work/rel" >/dev/null 2>&1 &
server_pid=$!
sleep 1

mirror_install() {
	dir="$1"
	shift
	HOME="$work/mhome" SHELL=/bin/zsh LEVELRAIL_VERSION=v9.9.9 LEVELRAIL_RELEASE_BASE_URL="http://127.0.0.1:8099/v9.9.9/$dir" \
		LEVELRAIL_INSECURE_MIRROR=1 "$@" sh "$script" 2>&1 || true
}
out="$(mirror_install tampered)"
echo "$out" | grep -q 'checksum mismatch' || fail "tampered checksums.txt was not rejected: $out"
out="$(mirror_install good)"
echo "$out" | grep -q 'Checksum verified' || fail "good checksum was not accepted: $out"

if sudo -n true 2>/dev/null; then
	echo "== sudo install goes to /usr/local/bin"
	sudo_installed=1
	sudo env LEVELRAIL_BINARY_FILE="$binary" sh "$script" >/dev/null 2>&1
	[ -x /usr/local/bin/levelrail-cli ] || fail "sudo install did not land in /usr/local/bin"
	/usr/local/bin/levelrail-cli version --help >/dev/null 2>&1 || fail "sudo-installed levelrail-cli version failed"
else
	echo "== sudo not available without a password, skipping the root install"
fi

echo "install-cli.sh passed on macOS"
