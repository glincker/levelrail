#!/usr/bin/env bash
# Throwaway DigitalOcean droplets for real-VPS QA: create, build once, push
# compressed and verified binaries, enroll agents, tear down. See README.md.
#
# Usage: scripts/qa/droplet.sh [--dry-run] <command> [args]
#   up N [--region R] [--size S] [--image I] [--runtime docker|podman]
#   build [--sha REF] [--arch amd64|arm64|both]
#   push [--key KEY] <ip>...
#   enroll [--api-port P]        needs QA_API_TOKEN in the environment
#   status | cost | down --yes
# Env: QA_SSH_KEY (private key path, default ~/.ssh/id_ed25519), QA_DO_SSH_KEY
#   (doctl key id or fingerprint, default: first key in the account),
#   QA_CACHE (default ~/.cache/levelrail-qa), QA_API_TOKEN (never written to disk).
set -euo pipefail

TAG="levelrail-qa"
DRY_RUN=0
CACHE="${QA_CACHE:-$HOME/.cache/levelrail-qa}"
SSH_KEY="${QA_SSH_KEY:-$HOME/.ssh/id_ed25519}"
MODULE="github.com/GLINCKER/levelrail"
ARTIFACTS=(levelrail levelrail-cli levelrail-agent)
SSH_OPTS=()

log() { printf '[qa] %s\n' "$*" >&2; }
die() {
	printf '[qa] error: %s\n' "$*" >&2
	exit 1
}

# Quote every word so a copy-paste of the printed line behaves like the run.
fmt_cmd() { printf '%q ' "$@"; }

# Side-effecting commands go through here so --dry-run shows them verbatim.
run() {
	if [[ "$DRY_RUN" == 1 ]]; then
		printf '+ %s\n' "$(fmt_cmd "$@")" >&2
		return 0
	fi
	"$@"
}

ssh_opts() {
	SSH_OPTS=(-i "$SSH_KEY" -o BatchMode=yes -o StrictHostKeyChecking=accept-new
		-o ConnectTimeout=15 -o ServerAliveInterval=15 -o ServerAliveCountMax=4)
}

rssh() {
	local ip="$1"
	shift
	ssh_opts
	run ssh "${SSH_OPTS[@]}" "root@$ip" "$@"
}

# Pure helpers (unit tested in droplet_test.sh).

valid_runtime() { [[ "$1" == docker || "$1" == podman ]]; }

valid_arch() { [[ "$1" == amd64 || "$1" == arm64 ]]; }

arch_of_uname() {
	case "$1" in
	x86_64) echo amd64 ;;
	aarch64 | arm64) echo arm64 ;;
	*) return 1 ;;
	esac
}

sha256_stdin() {
	if command -v sha256sum >/dev/null 2>&1; then sha256sum | awk '{print $1}'; else shasum -a 256 | awk '{print $1}'; fi
}

sha256_file() { sha256_stdin <"$1"; }

# Content address of a build: same sha, same dirty diff, same arch means same dir.
build_key() {
	local head="$1" diff_hash="$2" arch="$3"
	printf '%s|%s|%s|embedweb' "$head" "$diff_hash" "$arch" | sha256_stdin | cut -c1-16
}

# Sums the last column (hourly price) of "name price" rows on stdin: "count total".
sum_hourly() { awk '{ s += $NF; n++ } END { printf "%d %.5f\n", n, s }'; }

monthly_from_hourly() { awk -v h="$1" 'BEGIN { printf "%.2f\n", h * 730 }'; }

pick_compressor() {
	if command -v zstd >/dev/null 2>&1; then echo zstd; else echo gzip; fi
}

# Retry with linear backoff; ssh resets and flaky links are the norm.
retry() {
	local tries="$1" n=1
	shift
	until "$@"; do
		if ((n >= tries)); then return 1; fi
		log "attempt $n/$tries failed, retrying: $(fmt_cmd "$@")"
		sleep $((n * 3))
		n=$((n + 1))
	done
}

# doctl queries.

droplet_rows() {
	# Columns: ID Name PublicIPv4 PriceHourly. Dry run returns one placeholder row.
	local cmd=(doctl compute droplet list --tag-name "$TAG" --format "ID,Name,PublicIPv4,PriceHourly" --no-header)
	if [[ "$DRY_RUN" == 1 ]]; then
		printf '+ %s\n' "$(fmt_cmd "${cmd[@]}")" >&2
		echo "0 ${TAG}-1 192.0.2.1 0.00893"
		return 0
	fi
	"${cmd[@]}"
}

droplet_ips() { droplet_rows | awk 'NF >= 3 { print $3 }'; }

resolve_do_key() {
	if [[ -n "${QA_DO_SSH_KEY:-}" ]]; then
		echo "$QA_DO_SSH_KEY"
		return 0
	fi
	if [[ "$DRY_RUN" == 1 ]]; then
		echo "<first-key-id>"
		return 0
	fi
	local id
	id="$(doctl compute ssh-key list --format ID --no-header | head -n1)"
	[[ -n "$id" ]] || die "no ssh key in the DigitalOcean account; set QA_DO_SSH_KEY"
	echo "$id"
}

wait_ssh() {
	local ip="$1" i
	ssh_opts
	if [[ "$DRY_RUN" == 1 ]]; then
		printf '+ wait for ssh on %s (ssh true, up to 60 tries)\n' "$ip" >&2
		return 0
	fi
	for i in $(seq 1 60); do
		if ssh "${SSH_OPTS[@]}" "root@$ip" true 2>/dev/null; then return 0; fi
		sleep 5
	done
	die "ssh never came up on $ip"
}

install_runtime() {
	local ip="$1" runtime="$2" pkg=docker.io
	[[ "$runtime" == podman ]] && pkg=podman
	rssh "$ip" "export DEBIAN_FRONTEND=noninteractive; command -v $runtime >/dev/null || (apt-get update -qq && apt-get install -y -qq $pkg zstd rsync)"
}

cmd_up() {
	local n="${1:-}" region="${QA_REGION:-nyc3}" size="${QA_SIZE:-s-2vcpu-4gb}" image="${QA_IMAGE:-ubuntu-24-04-x64}" runtime=docker
	if ! [[ "$n" =~ ^[0-9]+$ ]] || ((n < 1 || n > 10)); then
		die "usage: up N (1 to 10) [--region R] [--size S] [--image I] [--runtime docker|podman]"
	fi
	shift
	while (($#)); do
		case "$1" in
		--region) region="${2:?}"; shift 2 ;;
		--size) size="${2:?}"; shift 2 ;;
		--image) image="${2:?}"; shift 2 ;;
		--runtime) runtime="${2:?}"; shift 2 ;;
		*) die "unknown flag for up: $1" ;;
		esac
	done
	valid_runtime "$runtime" || die "runtime must be docker or podman"
	local key stamp names=() i ip
	key="$(resolve_do_key)"
	stamp="$(date +%m%d%H%M)"
	for i in $(seq 1 "$n"); do names+=("${TAG}-${stamp}-${i}"); done
	run doctl compute droplet create "${names[@]}" --region "$region" --size "$size" --image "$image" \
		--ssh-keys "$key" --tag-name "$TAG" --wait
	while read -r ip; do
		[[ -n "$ip" ]] || continue
		wait_ssh "$ip"
		install_runtime "$ip" "$runtime"
	done < <(droplet_ips)
	log "droplets:"
	droplet_rows
}

# Build.

check_embedded_web() {
	[[ "$DRY_RUN" == 1 ]] && return 0
	[[ -s web/dist/index.html ]] || die "web/dist/index.html is missing or empty; refusing to build a control plane with a stub dashboard"
}

# A stub build is far smaller than one that embeds the dashboard.
verify_embedded_binary() {
	local bin="$1" min="${QA_MIN_CP_BYTES:-20000000}" size
	size="$(wc -c <"$bin" | tr -d ' ')"
	((size >= min)) || die "$bin is $size bytes, below $min: embedded web looks missing"
}

latest_dir() {
	local arch="$1" key
	[[ -f "$CACHE/latest-$arch" ]] || die "no build for $arch; run build first"
	key="$(<"$CACHE/latest-$arch")"
	echo "$CACHE/$key/$arch"
}

go_build() {
	local arch="$1" out="$2" pkg="$3" ld="$4"
	shift 4
	run env CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build "$@" -trimpath -ldflags "$ld" -o "$out" "$pkg"
}

cmd_build() {
	local ref="" arch_sel=amd64 arches=() arch
	while (($#)); do
		case "$1" in
		--sha) ref="${2:?}"; shift 2 ;;
		--arch) arch_sel="${2:?}"; shift 2 ;;
		*) die "unknown flag for build: $1" ;;
		esac
	done
	if [[ "$arch_sel" == both ]]; then
		arches=(amd64 arm64)
	else
		valid_arch "$arch_sel" || die "arch must be amd64, arm64 or both"
		arches=("$arch_sel")
	fi
	local root head diff_hash version
	root="$(git rev-parse --show-toplevel)"
	cd "$root"
	head="$(git rev-parse HEAD)"
	if [[ -n "$ref" ]]; then
		[[ "$(git rev-parse --verify "$ref^{commit}")" == "$head" ]] || die "--sha must be the checked out commit (check it out or use a worktree)"
	fi
	# Untracked files count too, otherwise a new file would reuse a stale build.
	diff_hash="$({ git diff HEAD; git ls-files --others --exclude-standard -z | xargs -0 cat; } | sha256_stdin)"
	version="$(bash scripts/git-version.sh)-qa-${head:0:8}"

	local web_done=0 key dir ld f
	for arch in "${arches[@]}"; do
		key="$(build_key "$head" "$diff_hash" "$arch")"
		dir="$CACHE/$key/$arch"
		if [[ -f "$dir/.complete" ]]; then
			log "cache hit $key ($arch), skipping build"
			run bash -c 'printf "%s\n" "$1" >"$2"' _ "$key" "$CACHE/latest-$arch"
			continue
		fi
		if [[ "$web_done" == 0 ]]; then
			log "building web"
			run bash -c 'cd web && { [ -d node_modules ] || npm ci; } && npx vite build >/dev/null'
			check_embedded_web
			web_done=1
		fi
		log "building linux/$arch $version"
		run mkdir -p "$dir"
		ld="-s -w -X ${MODULE}/internal/version.Version=${version}"
		# -tags embedweb is mandatory for the control plane or the dashboard is a stub.
		go_build "$arch" "$dir/levelrail" ./cmd/levelrail "$ld" -tags embedweb
		go_build "$arch" "$dir/levelrail-cli" ./cmd/levelrail-cli "$ld"
		go_build "$arch" "$dir/levelrail-agent" ./cmd/levelrail-agent "$ld"
		if [[ "$DRY_RUN" == 0 ]]; then
			verify_embedded_binary "$dir/levelrail"
			for f in "${ARTIFACTS[@]}"; do printf '%s  %s\n' "$(sha256_file "$dir/$f")" "$f"; done >"$dir/SHA256SUMS"
			: >"$dir/.complete"
			printf '%s\n' "$key" >"$CACHE/latest-$arch"
		fi
		log "built $dir"
	done
}

# Push.

remote_arch() {
	local ip="$1" u
	if [[ "$DRY_RUN" == 1 ]]; then
		echo amd64
		return 0
	fi
	ssh_opts
	u="$(ssh "${SSH_OPTS[@]}" "root@$ip" uname -m)"
	arch_of_uname "$u" || die "unsupported arch $u on $ip"
}

# Returns 0 when it replaced the binary, 1 when the remote already matched.
push_one() {
	local ip="$1" dir="$2" name="$3" want comp ext remote_sha stage=/root/.qa-stage blob rname decomp
	if [[ "$DRY_RUN" == 1 ]]; then want="SHA256_OF_${name}"; else want="$(sha256_file "$dir/$name")"; fi
	remote_sha="-"
	if [[ "$DRY_RUN" == 0 ]]; then
		remote_sha="$(rssh "$ip" "sha256sum /usr/local/bin/$name 2>/dev/null | cut -d' ' -f1" || true)"
	fi
	if [[ "$remote_sha" == "$want" ]]; then
		log "$ip $name unchanged, skipped"
		return 1
	fi
	comp="$(pick_compressor)"
	ext=zst
	decomp="zstd -dc"
	if [[ "$comp" == gzip ]]; then
		ext=gz
		decomp="gzip -dc"
	fi
	blob="$dir/$name.$ext"
	rname="$name.${want:0:12}.$ext"
	if [[ ! -s "$blob" ]]; then
		if [[ "$comp" == zstd ]]; then
			run zstd -q -3 -T0 -f -o "$blob" "$dir/$name"
		elif [[ "$DRY_RUN" == 1 ]]; then
			run gzip -c -6 "$dir/$name" ">$blob"
		else
			gzip -c -6 "$dir/$name" >"$blob"
		fi
	fi
	rssh "$ip" "mkdir -p $stage"
	ssh_opts
	# --append-verify resumes a cut transfer; the name carries the hash so a partial is never stale.
	retry 5 run rsync --partial --append-verify -e "ssh ${SSH_OPTS[*]}" "$blob" "root@$ip:$stage/$rname"
	# Decompress to a temp name, verify, then mv into place: the swap is atomic.
	rssh "$ip" "set -e; $decomp $stage/$rname >/usr/local/bin/.$name.new; got=\$(sha256sum /usr/local/bin/.$name.new | cut -d' ' -f1); if [ \"\$got\" != '$want' ]; then echo 'sha mismatch' >&2; exit 1; fi; chmod 0755 /usr/local/bin/.$name.new; mv -f /usr/local/bin/.$name.new /usr/local/bin/$name; find $stage -type f -name '$name.*' -delete"
	return 0
}

restart_services() {
	local ip="$1"
	rssh "$ip" "for u in levelrail levelrail-agent; do if systemctl list-unit-files \"\$u.service\" 2>/dev/null | grep -q \"\$u\"; then systemctl restart \"\$u\"; fi; done; true"
}

cmd_push() {
	local key="" ips=() arch dir ip name changed
	while (($#)); do
		case "$1" in
		--key) key="${2:?}"; shift 2 ;;
		*) ips+=("$1"); shift ;;
		esac
	done
	((${#ips[@]})) || die "usage: push [--key KEY] <ip>..."
	for ip in "${ips[@]}"; do
		arch="$(remote_arch "$ip")"
		if [[ -n "$key" ]]; then dir="$CACHE/$key/$arch"; elif [[ "$DRY_RUN" == 1 ]]; then dir="$CACHE/<latest>/$arch"; else dir="$(latest_dir "$arch")"; fi
		if [[ "$DRY_RUN" == 0 && ! -f "$dir/.complete" ]]; then die "$dir is not a complete build"; fi
		changed=0
		for name in "${ARTIFACTS[@]}"; do
			if push_one "$ip" "$dir" "$name"; then changed=1; fi
		done
		if [[ "$changed" == 1 ]]; then restart_services "$ip"; else log "$ip up to date, no restart"; fi
	done
}

# Enroll, status, cost, down.

# The token goes over stdin so it never lands in argv, shell history or a file.
cp_cli() {
	local cp="$1" port="$2"
	shift 2
	printf '%s\n' "${QA_API_TOKEN:-dry-run-token}" |
		rssh "$cp" "read -r APP_API_TOKEN; export APP_API_TOKEN APP_API_URL=http://127.0.0.1:$port; levelrail-cli $*"
}

cmd_enroll() {
	local port=8080 ips=() cp agent join i=1
	while (($#)); do
		case "$1" in
		--api-port) port="${2:?}"; shift 2 ;;
		*) die "unknown flag for enroll: $1" ;;
		esac
	done
	[[ "$DRY_RUN" == 1 || -n "${QA_API_TOKEN:-}" ]] || die "set QA_API_TOKEN (an API token from the control plane); it is sent over ssh stdin only"
	mapfile -t ips < <(droplet_ips)
	((${#ips[@]} >= 1)) || die "no droplets tagged $TAG"
	cp="${ips[0]}"
	log "control plane: $cp, agents: $((${#ips[@]} - 1))"
	for agent in "${ips[@]:1}"; do
		join="$(cp_cli "$cp" "$port" "nodes add --name qa-agent-$i" | sed -n 's/^[[:space:]]*\(sudo .*\)$/\1/p' | head -n1 || true)"
		if [[ "$DRY_RUN" == 1 ]]; then join="JOIN_COMMAND_FROM_NODES_ADD"; fi
		[[ -n "$join" ]] || die "could not read the join command from nodes add on $cp (output format changed?)"
		rssh "$agent" "$join"
		i=$((i + 1))
	done
	cp_cli "$cp" "$port" "nodes list"
}

cmd_status() {
	local ip
	droplet_rows
	while read -r ip; do
		[[ -n "$ip" ]] || continue
		log "$ip"
		rssh "$ip" "systemctl is-active levelrail levelrail-agent 2>&1 | paste -sd' ' -; sha256sum /usr/local/bin/levelrail /usr/local/bin/levelrail-agent 2>/dev/null | cut -c1-12 | paste -sd' ' -" || log "$ip unreachable"
	done < <(droplet_ips)
}

cmd_cost() {
	local rows n total
	rows="$(droplet_rows)"
	read -r n total < <(printf '%s\n' "$rows" | awk 'NF >= 4 { print $2, $4 }' | sum_hourly)
	printf '%s droplets, %s USD per hour, about %s USD per month if left on\n' "$n" "$total" "$(monthly_from_hourly "$total")"
}

cmd_down() {
	local yes=0
	while (($#)); do
		case "$1" in
		--yes) yes=1; shift ;;
		*) die "unknown flag for down: $1" ;;
		esac
	done
	log "droplets tagged $TAG that will be destroyed:"
	droplet_rows
	if [[ "$yes" != 1 ]]; then die "re-run with --yes to destroy the droplets above"; fi
	# Tag scoped delete: droplets without the tag are never matched.
	run doctl compute droplet delete --tag-name "$TAG" --force
}

usage() { sed -n '2,14p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; }

main() {
	local args=() a sub
	for a in "$@"; do
		if [[ "$a" == --dry-run ]]; then DRY_RUN=1; else args+=("$a"); fi
	done
	set -- "${args[@]+"${args[@]}"}"
	sub="${1:-}"
	if [[ -z "$sub" ]]; then
		usage
		exit 2
	fi
	shift
	case "$sub" in
	up) cmd_up "$@" ;;
	build) cmd_build "$@" ;;
	push) cmd_push "$@" ;;
	enroll) cmd_enroll "$@" ;;
	status) cmd_status ;;
	cost) cmd_cost ;;
	down) cmd_down "$@" ;;
	*) die "unknown command: $sub" ;;
	esac
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then main "$@"; fi
