package trafficexport

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"time"
)

const visitorIDHexLen = 32

// VisitorID returns an anonymous id for a client that is stable within one
// UTC day and unlinkable across days. secret is the server secret; an empty
// secret yields "" so an unsalted hash is never exported.
func VisitorID(secret []byte, day time.Time, ip, ua string) string {
	if len(secret) == 0 {
		return ""
	}
	// Per-day key means yesterday's ids cannot be recomputed or matched today.
	dayMAC := hmac.New(sha256.New, secret)
	dayMAC.Write([]byte("visitor-day:" + day.UTC().Format("2006-01-02")))
	mac := hmac.New(sha256.New, dayMAC.Sum(nil))
	mac.Write([]byte(ip))
	mac.Write([]byte{0})
	mac.Write([]byte(ua))
	return hex.EncodeToString(mac.Sum(nil))[:visitorIDHexLen]
}
