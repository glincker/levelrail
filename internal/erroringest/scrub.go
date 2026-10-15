package erroringest

import "strings"

// MaxRawBytes bounds the stored JSON of one event.
const MaxRawBytes = 64 << 10

const filtered = "[Filtered]"

var sensitiveKeys = []string{
	"authorization", "cookie", "password", "passwd", "secret", "token",
	"api_key", "apikey", "api-key", "private_key", "credential", "session",
}

func isSensitive(key string) bool {
	k := strings.ToLower(key)
	for _, s := range sensitiveKeys {
		if strings.Contains(k, s) {
			return true
		}
	}
	return false
}

// scrub filters the values of sensitive keys anywhere in a decoded event,
// including header lists sent as [name, value] pairs.
func scrub(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			if isSensitive(k) {
				x[k] = filtered
				continue
			}
			x[k] = scrub(val)
		}
		return x
	case []any:
		if len(x) == 2 {
			if name, ok := x[0].(string); ok && isSensitive(name) {
				return []any{name, filtered}
			}
		}
		for i := range x {
			x[i] = scrub(x[i])
		}
		return x
	}
	return v
}
