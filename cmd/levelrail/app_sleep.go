package main

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
)

var (
	wakeTokenOnce  sync.Once
	wakeTokenValue string
)

// appWakeToken is the per-process secret the ingress wake route presents to
// the API's wake hook. Both live in this process, so it never leaves memory.
func appWakeToken() string {
	wakeTokenOnce.Do(func() {
		b := make([]byte, 24)
		if _, err := rand.Read(b); err != nil {
			panic("app sleep: crypto/rand failed: " + err.Error())
		}
		wakeTokenValue = hex.EncodeToString(b)
	})
	return wakeTokenValue
}
