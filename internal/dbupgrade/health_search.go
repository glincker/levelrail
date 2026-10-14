package dbupgrade

import "github.com/GLINCKER/levelrail/internal/store"

// tcpGet is a bash /dev/tcp HTTP/1.0 GET: the Qdrant and Typesense images
// ship bash but no HTTP client. The key is expanded by builtins only.
const tcpGet = `get() { exec 3<>/dev/tcp/127.0.0.1/$1 || return 1; printf 'GET %s HTTP/1.0\r\nHost: 127.0.0.1\r\n%s: %s\r\n\r\n' "$2" "$3" "$4" >&3; cat <&3; exec 3<&-; }
`

// searchHealthCommand checks the engine answers its authenticated API and
// prints its version first, the same contract as HealthCommand.
func searchHealthCommand(engine string) []string {
	switch engine {
	case store.EngineMeilisearch:
		return []string{"sh", "-c", `out=$(printf 'header = "Authorization: Bearer %s"\n' "$MEILI_MASTER_KEY" | curl -sSf -K - http://127.0.0.1:7700/version) || exit 1
v=$(printf '%s' "$out" | sed -n 's/.*"pkgVersion":"\([^"]*\)".*/\1/p')
[ -n "$v" ] || { echo "no version in answer"; exit 1; }
echo "$v"`}
	case store.EngineQdrant:
		return []string{"bash", "-c", tcpGet + `out=$(get 6333 / api-key "$QDRANT__SERVICE__API_KEY") || exit 1
case "$out" in "HTTP/1."?" 200"*) ;; *) echo "unexpected answer"; exit 1;; esac
v=$(printf '%s' "$out" | sed -n 's/.*"version":"\([^"]*\)".*/\1/p')
[ -n "$v" ] || { echo "no version in answer"; exit 1; }
echo "$v"`}
	default:
		return []string{"bash", "-c", tcpGet + `out=$(get 8108 /debug X-TYPESENSE-API-KEY "$TYPESENSE_API_KEY") || exit 1
case "$out" in "HTTP/1."?" 200"*) ;; *) echo "unexpected answer"; exit 1;; esac
state=$(printf '%s' "$out" | sed -n 's/.*"state":\([0-9]*\).*/\1/p')
[ "$state" = "1" ] || { echo "node state is $state, not leader"; exit 1; }
v=$(printf '%s' "$out" | sed -n 's/.*"version":"\([^"]*\)".*/\1/p')
[ -n "$v" ] || { echo "no version in answer"; exit 1; }
echo "$v"`}
	}
}
