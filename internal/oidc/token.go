package oidc

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// TokenRequest is what a pipeline job's `oidc: {audience: ...}` config
// asks Manager.IssueToken to mint.
type TokenRequest struct {
	// Audience is the OIDC "aud" claim: the cloud provider's expected
	// value (e.g. "sts.amazonaws.com" for AWS, a GCP workload identity
	// pool audience URL, or a Vault role's bound_audiences entry).
	Audience string
	// Subject is the OIDC "sub" claim, the identity a trust policy
	// matches against. The caller builds it (pipeline jobs use
	// "repo:<app>:ref:<ref>:job:<job_key>"): this package stays
	// pipeline-agnostic, the same "translation is the caller's job"
	// reasoning internal/docker.ContainerSpec's own doc comments give.
	Subject string
	Repo    string
	Ref     string
	// PipelineID identifies the pipeline definition this run came from.
	PipelineID string
}

// claims is the JWT payload IssueToken signs: standard OIDC fields plus
// the handful of custom claims a trust policy can match on.
type claims struct {
	Issuer     string `json:"iss"`
	Subject    string `json:"sub"`
	Audience   string `json:"aud"`
	IssuedAt   int64  `json:"iat"`
	NotBefore  int64  `json:"nbf"`
	ExpiresAt  int64  `json:"exp"`
	Repo       string `json:"repo,omitempty"`
	Ref        string `json:"ref,omitempty"`
	PipelineID string `json:"pipeline_id,omitempty"`
}

type jwtHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
	Kid string `json:"kid"`
}

// sign builds and ES256-signs a compact JWS for c, the same hand-rolled
// stdlib-only pattern internal/githubapp's SignAppJWT uses for RS256:
// no JWT library dependency for one algorithm.
func (k *Key) sign(c claims) (string, error) {
	header, err := json.Marshal(jwtHeader{Alg: "ES256", Typ: "JWT", Kid: k.kid})
	if err != nil {
		return "", fmt.Errorf("oidc: marshal jwt header: %w", err)
	}
	body, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("oidc: marshal jwt claims: %w", err)
	}
	signingInput := base64URLEncode(header) + "." + base64URLEncode(body)

	digest := sha256.Sum256([]byte(signingInput))
	r, s, err := ecdsa.Sign(rand.Reader, k.private, digest[:])
	if err != nil {
		return "", fmt.Errorf("oidc: sign jwt: %w", err)
	}
	size := (k.private.Curve.Params().BitSize + 7) / 8
	sig := append(r.FillBytes(make([]byte, size)), s.FillBytes(make([]byte, size))...)

	return signingInput + "." + base64URLEncode(sig), nil
}

func base64URLEncode(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}
