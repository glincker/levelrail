package imagelife

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/distribution/reference"
)

const (
	cosignSigAnnotation = "dev.cosignproject.cosign/signature"
	maxBody             = 4 << 20
	manifestAccept      = "application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json"
)

// ErrNoSignature means the registry holds no signature for the image.
var ErrNoSignature = errors.New("no signature found for this image")

// Credentials authenticate registry reads.
type Credentials struct{ Username, Password string }

// Verifier checks cosign key-based signatures (ECDSA P-256) stored beside
// the image in its registry as a sha256-<digest>.sig tag.
type Verifier struct {
	HTTP *http.Client
}

// NewVerifier returns a Verifier with a bounded HTTP timeout.
func NewVerifier() *Verifier { return &Verifier{HTTP: &http.Client{Timeout: 20 * time.Second}} }

// ParsePublicKey reads a PEM encoded ECDSA public key.
func ParsePublicKey(pemData []byte) (*ecdsa.PublicKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, errors.New("public key is not PEM encoded")
	}
	k, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}
	pub, ok := k.(*ecdsa.PublicKey)
	if !ok {
		return nil, errors.New("only ECDSA public keys are supported")
	}
	return pub, nil
}

type manifest struct {
	Layers []struct {
		Digest      string            `json:"digest"`
		Annotations map[string]string `json:"annotations"`
	} `json:"layers"`
}

type simpleSigning struct {
	Critical struct {
		Image struct {
			Digest string `json:"docker-manifest-digest"`
		} `json:"image"`
	} `json:"critical"`
}

// Verify checks that ref (repo@sha256:...) carries a signature made by pub.
func (v *Verifier) Verify(ctx context.Context, ref string, pub *ecdsa.PublicKey, creds Credentials) error {
	named, err := reference.ParseNormalizedNamed(ref)
	if err != nil {
		return fmt.Errorf("parse image reference %q: %w", ref, err)
	}
	digested, ok := named.(reference.Digested)
	if !ok {
		return fmt.Errorf("image %q is not pinned to a digest", ref)
	}
	digest := digested.Digest().String()
	repo := reference.Path(named)
	host := reference.Domain(named)
	if host == "docker.io" {
		host = "registry-1.docker.io"
	}
	sigTag := strings.Replace(digest, ":", "-", 1) + ".sig"
	c := &regClient{http: v.HTTP, host: host, repo: repo, creds: creds}
	var m manifest
	if err := c.getJSON(ctx, "/v2/"+repo+"/manifests/"+sigTag, manifestAccept, &m); err != nil {
		return err
	}
	for _, l := range m.Layers {
		sigB64 := l.Annotations[cosignSigAnnotation]
		if sigB64 == "" {
			continue
		}
		payload, err := c.getBlob(ctx, l.Digest)
		if err != nil {
			return err
		}
		if verifyPayload(pub, payload, sigB64, digest) == nil {
			return nil
		}
	}
	return errors.New("no signature matched the configured public key")
}

func verifyPayload(pub *ecdsa.PublicKey, payload []byte, sigB64, digest string) error {
	sig, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}
	sum := sha256.Sum256(payload)
	if !ecdsa.VerifyASN1(pub, sum[:], sig) {
		return errors.New("signature does not verify")
	}
	var s simpleSigning
	if err := json.Unmarshal(payload, &s); err != nil {
		return fmt.Errorf("decode signed payload: %w", err)
	}
	if s.Critical.Image.Digest != digest {
		return errors.New("signature is for a different image digest")
	}
	return nil
}

// SignPayload builds the signed payload and signature for digest. It exists
// so tests and tooling can produce what Verify consumes.
func SignPayload(priv *ecdsa.PrivateKey, repo, digest string) (payload []byte, sigB64 string, err error) {
	payload, err = json.Marshal(map[string]any{
		"critical": map[string]any{
			"identity": map[string]string{"docker-reference": repo},
			"image":    map[string]string{"docker-manifest-digest": digest},
			"type":     "cosign container image signature",
		},
	})
	if err != nil {
		return nil, "", fmt.Errorf("encode payload: %w", err)
	}
	sum := sha256.Sum256(payload)
	sig, err := ecdsa.SignASN1(rand.Reader, priv, sum[:])
	if err != nil {
		return nil, "", fmt.Errorf("sign payload: %w", err)
	}
	return payload, base64.StdEncoding.EncodeToString(sig), nil
}

type regClient struct {
	http  *http.Client
	host  string
	repo  string
	creds Credentials
	token string
}

func (c *regClient) scheme() string {
	if strings.HasPrefix(c.host, "localhost") || strings.HasPrefix(c.host, "127.0.0.1") {
		return "http"
	}
	return "https"
}

func (c *regClient) do(ctx context.Context, path, accept string) ([]byte, error) {
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.scheme()+"://"+c.host+path, nil)
		if err != nil {
			return nil, fmt.Errorf("build registry request: %w", err)
		}
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		switch {
		case c.token != "":
			req.Header.Set("Authorization", "Bearer "+c.token)
		case c.creds.Username != "":
			req.SetBasicAuth(c.creds.Username, c.creds.Password)
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("registry request: %w", err)
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxBody))
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read registry response: %w", readErr)
		}
		switch {
		case resp.StatusCode == http.StatusNotFound:
			return nil, ErrNoSignature
		case resp.StatusCode == http.StatusUnauthorized && attempt == 0:
			if err := c.fetchToken(ctx, resp.Header.Get("Www-Authenticate")); err != nil {
				return nil, err
			}
			continue
		case resp.StatusCode != http.StatusOK:
			return nil, fmt.Errorf("registry answered %d", resp.StatusCode)
		}
		return body, nil
	}
	return nil, errors.New("registry refused authentication")
}

func (c *regClient) fetchToken(ctx context.Context, challenge string) error {
	params := parseChallenge(challenge)
	realm := params["realm"]
	if realm == "" {
		return errors.New("registry requires authentication")
	}
	q := url.Values{}
	if s := params["service"]; s != "" {
		q.Set("service", s)
	}
	q.Set("scope", "repository:"+c.repo+":pull")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, realm+"?"+q.Encode(), nil)
	if err != nil {
		return fmt.Errorf("build token request: %w", err)
	}
	if c.creds.Username != "" {
		req.SetBasicAuth(c.creds.Username, c.creds.Password)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("registry token request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("registry token endpoint answered %d", resp.StatusCode)
	}
	var tok struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&tok); err != nil {
		return fmt.Errorf("decode registry token: %w", err)
	}
	c.token = tok.Token
	if c.token == "" {
		c.token = tok.AccessToken
	}
	return nil
}

func parseChallenge(h string) map[string]string {
	out := map[string]string{}
	h = strings.TrimPrefix(h, "Bearer ")
	for _, part := range strings.Split(h, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok {
			out[k] = strings.Trim(v, `"`)
		}
	}
	return out
}

func (c *regClient) getJSON(ctx context.Context, path, accept string, out any) error {
	b, err := c.do(ctx, path, accept)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("decode registry response: %w", err)
	}
	return nil
}

func (c *regClient) getBlob(ctx context.Context, digest string) ([]byte, error) {
	b, err := c.do(ctx, "/v2/"+c.repo+"/blobs/"+digest, "")
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(b)
	if "sha256:"+hex.EncodeToString(sum[:]) != digest {
		return nil, errors.New("signature blob does not match its digest")
	}
	return b, nil
}
