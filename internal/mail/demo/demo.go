// Package demo is a dev-only, in-memory stand-in for a mail server and its DNS
// zone, so the Mail page can be exercised without a container, provider or domain.
package demo

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"os"
	"sync"
)

const (
	// DevModeEnv is the existing dev gate.
	DevModeEnv = "APP_DEV_MODE"
	// EnableEnv is the extra opt-in.
	EnableEnv = "APP_MAIL_DEMO"

	// DefaultDomain is used when the caller types no domain.
	DefaultDomain = "demo.example"
	// AppName is the fake mail app every demo request resolves to.
	AppName = "mail-demo"
	// ProviderName labels the fake zone provider in responses and audit rows.
	ProviderName = "demo"
	// FakeMarker is a placeholder that is not a credential for anything.
	FakeMarker = "demo-not-a-real-value"
	// DKIMSelector is the demo signing selector.
	DKIMSelector = "demo2026"
	// MailIPv4 is a documentation address (RFC 5737), never routable.
	MailIPv4 = "203.0.113.10"
)

// Enabled reports whether demo mode is on. It needs both the dev gate and the
// opt-in; ignored is true when the opt-in is set without the dev gate.
func Enabled(devMode bool, getenv func(string) string) (on, ignored bool) {
	want := getenv(EnableEnv) == "1"
	return want && devMode, want && !devMode
}

var dkimKey = sync.OnceValue(func() string {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "ZGVtby1ub3QtYS1yZWFsLWtleQ=="
	}
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return "ZGVtby1ub3QtYS1yZWFsLWtleQ=="
	}
	return base64.StdEncoding.EncodeToString(der)
})

// DKIMPublicKey is a throwaway public key made per process; its private half is discarded.
func DKIMPublicKey() string { return dkimKey() }

// EnabledFromEnv is Enabled over the process environment.
func EnabledFromEnv(devMode bool) (on, ignored bool) { return Enabled(devMode, os.Getenv) }
