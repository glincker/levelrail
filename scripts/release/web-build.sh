#!/usr/bin/env bash
# Single source of truth for the release frontend build, used by release.yml and the PR dry run.
set -euo pipefail
root="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$root/web"

# The tsc step overran the default heap on tagged builds, so the cap is explicit and tunable.
export NODE_OPTIONS="--max-old-space-size=${RELEASE_NODE_HEAP_MB:-8192}"

if [ ! -d node_modules ]; then
	npm ci --ignore-scripts
fi

timer=()
if [ -x /usr/bin/time ]; then
	case "$(uname -s)" in
	Darwin) timer=(/usr/bin/time -l) ;;
	*) timer=(/usr/bin/time -v) ;;
	esac
fi

# Peak memory and wall time go to a file so the log stays short.
stats="$(mktemp)"
trap 'rm -f "$stats"' EXIT
if [ ${#timer[@]} -gt 0 ]; then
	"${timer[@]}" -o "$stats" npm run build >/dev/null
	grep -Ei 'maximum resident|peak memory|elapsed|real' "$stats" | head -3 || true
else
	npm run build >/dev/null
fi

test -s dist/index.html
echo "web build ok (heap cap ${RELEASE_NODE_HEAP_MB:-8192} MB)"
