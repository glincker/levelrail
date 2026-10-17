// Command signer creates an ed25519 publisher key and signs a plugin manifest.
//
//	go run ./test/fixtures/plugin-notify/signer -manifest plugin.yaml -key publisher.key
//
// With no key file it writes a new one (base64 seed, mode 0600). It prints the
// public key to trust, the key id to install with, and the detached signature.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/GLINCKER/levelrail/internal/plugin"
)

func main() {
	manifestPath := flag.String("manifest", "plugin.yaml", "manifest to sign")
	keyPath := flag.String("key", "publisher.key", "private key file, created if missing")
	flag.Parse()
	if err := run(*manifestPath, *keyPath); err != nil {
		fmt.Fprintln(os.Stderr, "signer:", err)
		os.Exit(1)
	}
}

func run(manifestPath, keyPath string) error {
	raw, err := os.ReadFile(manifestPath) //nolint:gosec // publisher's own manifest
	if err != nil {
		return err
	}
	man, err := plugin.ParseManifest(raw)
	if err != nil {
		return err
	}
	priv, err := loadOrCreateKey(keyPath)
	if err != nil {
		return err
	}
	pub := priv.Public().(ed25519.PublicKey)
	fmt.Printf("public_key: %s\nkey_id:     %s\nsignature:  %s\n",
		base64.StdEncoding.EncodeToString(pub), plugin.Fingerprint(pub), plugin.Sign(priv, raw, man.Digest()))
	return nil
}

func loadOrCreateKey(path string) (ed25519.PrivateKey, error) {
	if b, err := os.ReadFile(path); err == nil { //nolint:gosec // publisher's own key file
		seed, derr := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
		if derr != nil || len(seed) != ed25519.SeedSize {
			return nil, fmt.Errorf("%s is not a base64 ed25519 seed", path)
		}
		return ed25519.NewKeyFromSeed(seed), nil
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(priv.Seed())+"\n"), 0o600); err != nil {
		return nil, err
	}
	return priv, nil
}
