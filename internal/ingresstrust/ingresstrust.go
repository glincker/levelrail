// Package ingresstrust holds the per-process secret that marks requests the
// embedded ingress forwards to the control plane, so the API can trust the
// hop without depending on the TCP peer address (NAT may rewrite it).
package ingresstrust

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"sync"
)

// HeaderName carries the secret on the ingress to control plane hop. The API
// verifies and strips it before any handler runs.
const HeaderName = "X-Ingress-Hop-Token"

var (
	once   sync.Once
	secret string
)

// Secret returns this process's random marker value, created on first use.
func Secret() string {
	once.Do(func() {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			panic("ingresstrust: random source unavailable: " + err.Error())
		}
		secret = hex.EncodeToString(b)
	})
	return secret
}

// Valid reports whether v is this process's marker, in constant time.
func Valid(v string) bool {
	return v != "" && subtle.ConstantTimeCompare([]byte(v), []byte(Secret())) == 1
}
