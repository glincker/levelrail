package vault

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	vaultapi "github.com/hashicorp/vault/api"

	"github.com/glincker/levelrail/internal/secretmask"
	"github.com/glincker/levelrail/internal/store"
)

const (
	stsDefaultURL = "https://sts.amazonaws.com/"
	stsBody       = "Action=GetCallerIdentity&Version=2011-06-15"
	serverIDHdr   = "X-Vault-AWS-IAM-Server-ID"
)

// AWSLoginSpec names the Vault aws auth mount and role Levelrail logs in to with its own AWS identity.
type AWSLoginSpec struct {
	Address   string
	Namespace string
	Mount     string
	Role      string
	ServerID  string
	// STSURL overrides the signed endpoint, for tests only.
	STSURL string
}

func (s AWSLoginSpec) mount() string {
	if m := strings.Trim(s.Mount, "/"); m != "" {
		return m
	}
	return "aws"
}

type loginEntry struct {
	token   string
	renewAt time.Time
}

// AWSLogin caches Vault tokens obtained through auth/aws/login so a lease read does not log in every time.
type AWSLogin struct {
	tunables func(context.Context) Tunables
	now      func() time.Time
	mu       sync.Mutex
	cache    map[string]loginEntry
}

// NewAWSLogin returns a login helper reading its client bounds from tunables on every call.
func NewAWSLogin(tunables func(context.Context) Tunables) *AWSLogin {
	return &AWSLogin{tunables: tunables, now: time.Now, cache: map[string]loginEntry{}}
}

// signedIdentityRequest builds the signed sts:GetCallerIdentity request Vault replays to prove the caller.
func signedIdentityRequest(ctx context.Context, spec AWSLoginSpec, k AWSCredentials, now time.Time) (map[string]any, error) {
	url := spec.STSURL
	if url == "" {
		url = stsDefaultURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(stsBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=utf-8")
	if spec.ServerID != "" {
		req.Header.Set(serverIDHdr, spec.ServerID)
	}
	sum := sha256.Sum256([]byte(stsBody))
	creds := aws.Credentials{AccessKeyID: k.AccessKeyID, SecretAccessKey: k.SecretAccessKey, SessionToken: k.SessionToken}
	if err := v4.NewSigner().SignHTTP(ctx, creds, req, hex.EncodeToString(sum[:]), "sts", "us-east-1", now); err != nil {
		return nil, errors.New("vault: sign sts request failed")
	}
	hdrs := map[string][]string{}
	for name, vals := range req.Header {
		hdrs[name] = vals
	}
	rawHdrs, _ := json.Marshal(hdrs)
	b64 := base64.StdEncoding.EncodeToString
	return map[string]any{
		"role":                    spec.Role,
		"iam_http_request_method": http.MethodPost,
		"iam_request_url":         b64([]byte(url)),
		"iam_request_body":        b64([]byte(stsBody)),
		"iam_request_headers":     b64(rawHdrs),
	}, nil
}

// Token returns a live Vault token for the spec, logging in only when none is cached or it is past its
// renewal point. keys is called only when a login is needed.
func (l *AWSLogin) Token(ctx context.Context, spec AWSLoginSpec, keys func(context.Context) (AWSCredentials, error)) (string, error) {
	if spec.Role == "" {
		return "", errors.New("vault: aws login role is required")
	}
	ck := strings.Join([]string{spec.Address, spec.Namespace, spec.mount(), spec.Role, spec.ServerID, spec.STSURL}, "|")
	l.mu.Lock()
	e, ok := l.cache[ck]
	l.mu.Unlock()
	if ok && l.now().Before(e.renewAt) {
		return e.token, nil
	}
	tun := l.tunables(ctx)
	hc, err := guardedClient(spec.Address, tun.AllowPrivate, tun.Timeout)
	if err != nil {
		return "", err
	}
	c, err := vaultapi.NewClient(&vaultapi.Config{Address: spec.Address, HttpClient: hc})
	if err != nil {
		return "", fmt.Errorf("vault: build client: %w", err)
	}
	if spec.Namespace != "" {
		c.SetNamespace(spec.Namespace)
	}
	k, err := keys(ctx)
	if err != nil {
		return "", fmt.Errorf("vault: no aws identity for login: %w", err)
	}
	secretmask.Add(k.SecretAccessKey)
	secretmask.Add(k.SessionToken)
	data, err := signedIdentityRequest(ctx, spec, k, l.now())
	if err != nil {
		return "", err
	}
	sec, err := c.Logical().WriteWithContext(ctx, "auth/"+spec.mount()+"/login", data)
	if err != nil {
		return "", fmt.Errorf("vault: aws login: %w", err)
	}
	if sec == nil || sec.Auth == nil || sec.Auth.ClientToken == "" {
		return "", errors.New("vault: aws login returned no client token")
	}
	secretmask.Add(sec.Auth.ClientToken)
	next := loginEntry{token: sec.Auth.ClientToken}
	if lease := time.Duration(sec.Auth.LeaseDuration) * time.Second; lease > 0 {
		next.renewAt = l.now().Add(lease * time.Duration(min(max(tun.RenewPercent, 10), 95)) / 100)
	} else {
		next.renewAt = l.now().Add(time.Minute)
	}
	l.mu.Lock()
	l.cache[ck] = next
	l.mu.Unlock()
	return next.token, nil
}

// Probe checks, with the given token, that Vault answers and the aws secrets engine role exists.
// The caller gets a class of failure, never Vault's own text.
func (b *AWSBroker) Probe(ctx context.Context, s AWSSpec, token string) error {
	c, err := b.client(ctx, s, token)
	if err != nil {
		return err
	}
	if _, err := c.Sys().HealthWithContext(ctx); err != nil {
		return fmt.Errorf("vault: unreachable: %w", err)
	}
	sec, err := c.Logical().ReadWithContext(ctx, s.mount()+"/roles/"+s.Role)
	if err != nil {
		return fmt.Errorf("vault: read aws role: %w", err)
	}
	if sec == nil {
		return ErrNoAWSRole
	}
	return nil
}

// ErrNoAWSRole marks an aws secrets engine role that does not exist at Vault.
var ErrNoAWSRole = errors.New("vault: aws role not found")

// SessionToken returns the Vault token for cfg: the credential for token auth or a cached AppRole login.
func (r *Resolver) SessionToken(ctx context.Context, cfg store.VaultSettings, credential string) (string, error) {
	tun := r.tunables(ctx)
	secretmask.Add(credential)
	hc, err := guardedClient(cfg.Address, tun.AllowPrivate, tun.Timeout)
	if err != nil {
		return "", err
	}
	c, err := vaultapi.NewClient(&vaultapi.Config{Address: cfg.Address, HttpClient: hc})
	if err != nil {
		return "", fmt.Errorf("vault: build client: %w", err)
	}
	if cfg.Namespace != "" {
		c.SetNamespace(cfg.Namespace)
	}
	return r.token(ctx, c, cfg, credential, fingerprint(cfg, credential), tun)
}
