package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
)

// gitStateSep joins the connection id and PKCE verifier stored under one
// OAuth state value. Neither part can contain a newline.
const gitStateSep = "\n"

// newGitPKCEVerifier returns a 43 character RFC 7636 verifier from 32 random bytes.
func newGitPKCEVerifier() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate pkce verifier: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func gitPKCEChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func joinGitStateValue(connID, verifier string) string {
	return connID + gitStateSep + verifier
}

func splitGitStateValue(v string) (connID, verifier string) {
	connID, verifier, _ = strings.Cut(v, gitStateSep)
	return connID, verifier
}
