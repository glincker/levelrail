package secrets

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// TransitName is the backend name stored in a wrapped master key.
const TransitName = "vault-transit"

const transitTimeout = 15 * time.Second

// VaultTransit wraps keys with a HashiCorp Vault or OpenBao Transit engine
// over its HTTP API, so no SDK dependency is needed.
type VaultTransit struct {
	Addr      string
	Token     string
	Key       string
	Mount     string
	Namespace string
	Client    *http.Client
}

// NewVaultTransit validates the config. Plain HTTP is allowed only to a loopback address.
func NewVaultTransit(addr, token, key, mount, namespace string) (*VaultTransit, error) {
	addr = strings.TrimRight(strings.TrimSpace(addr), "/")
	u, err := url.Parse(addr)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("secrets: vault address %q is not a valid URL", addr)
	}
	if u.Scheme != "https" && (u.Scheme != "http" || !isLoopbackHost(u.Hostname())) {
		return nil, errors.New("secrets: vault address must use https (plain http is only allowed to localhost)")
	}
	if strings.TrimSpace(token) == "" || strings.TrimSpace(key) == "" {
		return nil, errors.New("secrets: vault token and transit key name are required")
	}
	if mount = strings.Trim(strings.TrimSpace(mount), "/"); mount == "" {
		mount = "transit"
	}
	return &VaultTransit{Addr: addr, Token: strings.TrimSpace(token), Key: strings.TrimSpace(key), Mount: mount, Namespace: strings.TrimSpace(namespace)}, nil
}

func isLoopbackHost(h string) bool {
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// Name implements KeyWrapper.
func (v *VaultTransit) Name() string { return TransitName }

// Wrap implements KeyWrapper.
func (v *VaultTransit) Wrap(ctx context.Context, plaintext []byte) (string, error) {
	var out struct {
		Data struct {
			Ciphertext string `json:"ciphertext"`
		} `json:"data"`
	}
	body := map[string]string{"plaintext": base64.StdEncoding.EncodeToString(plaintext)}
	if err := v.call(ctx, "encrypt", body, &out); err != nil {
		return "", err
	}
	if out.Data.Ciphertext == "" {
		return "", errors.New("vault returned no ciphertext")
	}
	return out.Data.Ciphertext, nil
}

// Unwrap implements KeyWrapper.
func (v *VaultTransit) Unwrap(ctx context.Context, ciphertext string) ([]byte, error) {
	var out struct {
		Data struct {
			Plaintext string `json:"plaintext"`
		} `json:"data"`
	}
	if err := v.call(ctx, "decrypt", map[string]string{"ciphertext": ciphertext}, &out); err != nil {
		return nil, err
	}
	pt, err := base64.StdEncoding.DecodeString(out.Data.Plaintext)
	if err != nil || len(pt) == 0 {
		return nil, errors.New("vault returned an unreadable plaintext")
	}
	return pt, nil
}

func (v *VaultTransit) call(ctx context.Context, op string, body any, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, transitTimeout)
	defer cancel()
	endpoint := fmt.Sprintf("%s/v1/%s/%s/%s", v.Addr, v.Mount, op, url.PathEscape(v.Key))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("X-Vault-Token", v.Token)
	req.Header.Set("Content-Type", "application/json")
	if v.Namespace != "" {
		req.Header.Set("X-Vault-Namespace", v.Namespace)
	}
	c := v.Client
	if c == nil {
		c = &http.Client{Timeout: transitTimeout}
	}
	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("reach vault: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read vault response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("vault %s returned %d: %s", op, resp.StatusCode, vaultErrors(data))
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode vault response: %w", err)
	}
	return nil
}

func vaultErrors(data []byte) string {
	var e struct {
		Errors []string `json:"errors"`
	}
	if json.Unmarshal(data, &e) == nil && len(e.Errors) > 0 {
		return strings.Join(e.Errors, "; ")
	}
	return "no error detail"
}
