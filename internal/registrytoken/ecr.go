package registrytoken

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"

	"github.com/GLINCKER/levelrail/internal/provision"
)

const ecrTarget = "AmazonEC2ContainerRegistry_V20150921.GetAuthorizationToken"

// ECR mints a password with ECR GetAuthorizationToken, signed with the same
// AWS identity modes EC2 provisioning uses (static key, assumed role, ambient).
type ECR struct {
	Client *http.Client
	// Endpoint overrides the regional API URL, for tests.
	Endpoint string
	Now      func() time.Time
}

// Mint implements Minter.
func (e *ECR) Mint(ctx context.Context, cloudSecret string, cfg Config) (Minted, error) {
	creds, err := provision.ParseAWSCredentials(cloudSecret)
	if err != nil {
		return Minted{}, fmt.Errorf("registrytoken: ecr: %w", err)
	}
	region := cfg.Region
	if region == "" {
		region = creds.DefaultRegion()
	}
	provider, err := creds.CredentialsProvider(ctx, region)
	if err != nil {
		return Minted{}, fmt.Errorf("registrytoken: ecr: %w", err)
	}
	cred, err := provider.Retrieve(ctx)
	if err != nil {
		return Minted{}, fmt.Errorf("registrytoken: ecr: resolve aws credentials: %w", err)
	}

	endpoint := e.Endpoint
	if endpoint == "" {
		endpoint = "https://api.ecr." + region + ".amazonaws.com/"
	}
	body := []byte("{}")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Minted{}, fmt.Errorf("registrytoken: ecr: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", ecrTarget)
	sum := sha256.Sum256(body)
	now := time.Now()
	if e.Now != nil {
		now = e.Now()
	}
	if err := v4.NewSigner().SignHTTP(ctx, cred, req, hex.EncodeToString(sum[:]), "ecr", region, now); err != nil {
		return Minted{}, fmt.Errorf("registrytoken: ecr: sign request: %w", err)
	}

	resp, err := e.Client.Do(req)
	if err != nil {
		return Minted{}, fmt.Errorf("registrytoken: ecr: get authorization token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Minted{}, statusError("ecr get authorization token", resp)
	}
	var out struct {
		AuthorizationData []struct {
			AuthorizationToken string  `json:"authorizationToken"`
			ExpiresAt          float64 `json:"expiresAt"`
			ProxyEndpoint      string  `json:"proxyEndpoint"`
		} `json:"authorizationData"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return Minted{}, fmt.Errorf("registrytoken: ecr: decode response: %w", err)
	}
	if len(out.AuthorizationData) == 0 {
		return Minted{}, fmt.Errorf("registrytoken: ecr: response had no authorization data")
	}
	d := out.AuthorizationData[0]
	raw, err := base64.StdEncoding.DecodeString(d.AuthorizationToken)
	if err != nil {
		return Minted{}, fmt.Errorf("registrytoken: ecr: decode authorization token: %w", err)
	}
	user, pass, ok := strings.Cut(string(raw), ":")
	if !ok || pass == "" {
		return Minted{}, fmt.Errorf("registrytoken: ecr: authorization token has unexpected shape")
	}
	host := cfg.RegistryHost
	if host == "" {
		host = strings.TrimPrefix(strings.TrimPrefix(d.ProxyEndpoint, "https://"), "http://")
	}
	if host == "" {
		return Minted{}, fmt.Errorf("registrytoken: ecr: registry host is required")
	}
	sec := int64(d.ExpiresAt)
	return Minted{
		RegistryHost: host,
		Username:     user,
		Password:     pass,
		ExpiresAt:    time.Unix(sec, 0).UTC(),
	}, nil
}
