package egress

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"net/url"
)

// SecretHash is the SHA-256 of a plugin credential. Only hashes are stored.
type SecretHash [sha256.Size]byte

// HashSecret returns the stored form of a secret.
func HashSecret(secret string) SecretHash { return sha256.Sum256([]byte(secret)) }

// CredentialStore verifies a plugin's proxy credential.
type CredentialStore interface {
	Verify(plugin, secret string) bool
}

// HashedCredentials is a CredentialStore over plugin to secret hash.
type HashedCredentials struct {
	hashes map[string]SecretHash
}

// NewHashedCredentials copies the map so later edits cannot change it.
func NewHashedCredentials(hashes map[string]SecretHash) *HashedCredentials {
	c := make(map[string]SecretHash, len(hashes))
	for k, v := range hashes {
		c[k] = v
	}
	return &HashedCredentials{hashes: c}
}

// Verify compares in constant time and does the same work for an unknown
// plugin, so timing does not reveal which names exist.
func (h *HashedCredentials) Verify(plugin, secret string) bool {
	want, known := h.hashes[plugin]
	got := HashSecret(secret)
	eq := subtle.ConstantTimeCompare(want[:], got[:]) == 1
	return known && eq
}

// NewCredential returns a random secret and the hash to store for it.
func NewCredential() (secret string, hash SecretHash, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", SecretHash{}, err
	}
	secret = base64.RawURLEncoding.EncodeToString(b)
	return secret, HashSecret(secret), nil
}

// ProxyURL builds the proxy URL a plugin gets in its proxy environment.
func ProxyURL(plugin, secret, hostport string) string {
	u := url.URL{Scheme: "http", User: url.UserPassword(plugin, secret), Host: hostport}
	return u.String()
}

// proxyCredentials reads Basic credentials from Proxy-Authorization.
func proxyCredentials(r *http.Request) (user, pass string, ok bool) {
	v := r.Header.Get("Proxy-Authorization")
	if v == "" {
		return "", "", false
	}
	return (&http.Request{Header: http.Header{"Authorization": {v}}}).BasicAuth()
}
