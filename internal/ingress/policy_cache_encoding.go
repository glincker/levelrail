package ingress

import (
	"net/http"
	"strconv"
	"strings"
)

const encodingIdentity = "identity"

type encodingPref struct {
	name string
	q    float64
}

// parseAcceptEncoding returns the listed codings with their q values and the
// q of a "*" entry (-1 when absent). Malformed q values count as 1.
func parseAcceptEncoding(values []string) (prefs []encodingPref, star float64) {
	star = -1
	for _, v := range values {
		for _, part := range strings.Split(v, ",") {
			fields := strings.Split(part, ";")
			name := strings.ToLower(strings.TrimSpace(fields[0]))
			if name == "" {
				continue
			}
			q := 1.0
			for _, p := range fields[1:] {
				k, val, ok := strings.Cut(strings.TrimSpace(p), "=")
				if ok && strings.EqualFold(strings.TrimSpace(k), "q") {
					if f, err := strconv.ParseFloat(strings.TrimSpace(val), 64); err == nil && f >= 0 && f <= 1 {
						q = f
					}
				}
			}
			if name == "*" {
				star = q
				continue
			}
			prefs = append(prefs, encodingPref{name, q})
		}
	}
	return prefs, star
}

// codingQ is the q the client gives name: its own entry, else "*", else 0.
func codingQ(prefs []encodingPref, star float64, name string) float64 {
	for _, p := range prefs {
		if p.name == name {
			return p.q
		}
	}
	if star >= 0 {
		return star
	}
	return 0
}

// negotiatedEncoding names the coding the response for these Accept-Encoding
// values is stored under: the highest q of the encoder's own codings (ties
// follow prefer order), else br for an upstream that compresses itself, else
// identity. Browser strings that differ only in order, spacing or q values
// the encoder ignores therefore share one entry.
func negotiatedEncoding(values []string, prefer []string) string {
	prefs, star := parseAcceptEncoding(values)
	best, bestQ := "", 0.0
	for _, name := range prefer {
		if q := codingQ(prefs, star, name); q > bestQ {
			best, bestQ = name, q
		}
	}
	if best != "" {
		return best
	}
	if codingQ(prefs, star, "br") > 0 {
		return "br"
	}
	return encodingIdentity
}

// acceptsContentEncoding reports whether a client may receive a body encoded
// as ce. It guards stored entries, since an upstream may compress on its own.
func acceptsContentEncoding(values []string, ce string) bool {
	ce = strings.ToLower(strings.TrimSpace(ce))
	if ce == "" || ce == encodingIdentity {
		return true
	}
	prefs, star := parseAcceptEncoding(values)
	return codingQ(prefs, star, ce) > 0
}

func entryAcceptable(e *cacheEntry, r *http.Request) bool {
	return acceptsContentEncoding(r.Header.Values("Accept-Encoding"), e.header.Get("Content-Encoding"))
}
