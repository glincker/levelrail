#!/usr/bin/env bash
# Sets the Docker Hub short and full description for one image.
# Usage: scripts/release/dockerhub-describe.sh <image> <tag> [--dry-run]
# Env: DOCKERHUB_USERNAME, DOCKERHUB_TOKEN, DOCKERHUB_NAMESPACE (default levelrail).
# Never fails the release: API errors exit 0 with a warning.
set -uo pipefail

image="${1:?usage: dockerhub-describe.sh <levelrail|levelrail-agent> <tag> [--dry-run]}"
tag="${2:?tag}"
dry_run="${3:-}"
ns="${DOCKERHUB_NAMESPACE:-levelrail}"
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
overview="$root/docs/dockerhub/$image.md"

case "$image" in
levelrail) short="Self-hosted PaaS control plane: git push deploys, automatic HTTPS, logs and rollback." ;;
levelrail-agent) short="Levelrail node agent: run deployments on extra servers from one control plane." ;;
*) echo "dockerhub-describe: unknown image $image" >&2; exit 2 ;;
esac
[ -f "$overview" ] || { echo "dockerhub-describe: missing $overview" >&2; exit 2; }
if [ "${#short}" -gt 100 ]; then
	echo "dockerhub-describe: short description is ${#short} chars, max 100" >&2
	exit 2
fi

payload="$(jq -n --arg short "$short" --arg full "$(sed "s|@TAG@|$tag|g" "$overview")" \
	'{description: $short, full_description: $full}')"
if [ "$dry_run" = "--dry-run" ]; then
	echo "$payload"
	exit 0
fi

warn() { echo "::warning::dockerhub-describe: $1, skipping"; exit 0; }
[ -n "${DOCKERHUB_USERNAME:-}" ] && [ -n "${DOCKERHUB_TOKEN:-}" ] || warn "credentials missing"

jwt="$(jq -n --arg u "$DOCKERHUB_USERNAME" --arg p "$DOCKERHUB_TOKEN" '{username: $u, password: $p}' |
	curl -fsS -H 'Content-Type: application/json' --data @- https://hub.docker.com/v2/users/login | jq -r .token)" || warn "login failed"
[ -n "$jwt" ] && [ "$jwt" != null ] || warn "login returned no token"
echo "::add-mask::$jwt"

# Header goes through a curl config on stdin so the token never lands in argv.
printf 'header = "Authorization: JWT %s"\n' "$jwt" |
	curl -fsS -X PATCH -K - -H 'Content-Type: application/json' --data "$payload" \
		"https://hub.docker.com/v2/repositories/$ns/$image/" >/dev/null || warn "update failed"
echo "dockerhub-describe: updated $ns/$image"
exit 0
