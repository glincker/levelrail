package cloudconn

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/GLINCKER/levelrail/internal/secretmask"
)

const (
	ssoGrantDevice  = "urn:ietf:params:oauth:grant-type:device_code"
	ssoGrantRefresh = "refresh_token"
	ssoScope        = "sso:account:access"
)

var (
	// ErrSSONeedsRelogin means the refresh path is gone; a person must sign in again.
	ErrSSONeedsRelogin = errors.New("cloudconn: aws sso session ended, sign in again")
	// ErrSSOPending is the normal device flow answer while the person has not approved yet.
	ErrSSOPending = errors.New("cloudconn: aws sso authorization pending")
	// ErrSSOSlowDown asks the caller to poll less often.
	ErrSSOSlowDown = errors.New("cloudconn: aws sso slow down")
	// ErrSSODenied covers an expired or refused device code.
	ErrSSODenied = errors.New("cloudconn: aws sso authorization ended")

	ssoSettingsFn atomic.Pointer[func() SSOSettings]

	ssoRoleRe = regexp.MustCompile(`^[A-Za-z0-9+=,.@_-]{1,64}$`)
)

// SSOSettings are the live tunables of the aws_sso path; the API layer feeds them from runtime settings.
type SSOSettings struct {
	RenewPercent     int
	TokenSkewSeconds int
	MaxAccounts      int
	TimeoutSeconds   int
	MaxResponseBytes int
}

// DefaultSSOSettings is what a broker uses until a settings source is registered.
func DefaultSSOSettings() SSOSettings {
	return SSOSettings{RenewPercent: 80, TokenSkewSeconds: 60, MaxAccounts: 100, TimeoutSeconds: 15, MaxResponseBytes: 4 << 20}
}

// SetSSOSettings registers the live source, read again on every call so a change applies at once.
func SetSSOSettings(fn func() SSOSettings) { ssoSettingsFn.Store(&fn) }

// SSOCurrentSettings returns the live values with out of range ones replaced by the defaults.
func SSOCurrentSettings() SSOSettings {
	d := DefaultSSOSettings()
	f := ssoSettingsFn.Load()
	if f == nil {
		return d
	}
	s := (*f)()
	if s.RenewPercent < 1 || s.RenewPercent > 99 {
		s.RenewPercent = d.RenewPercent
	}
	if s.TokenSkewSeconds < 0 {
		s.TokenSkewSeconds = d.TokenSkewSeconds
	}
	if s.MaxAccounts < 1 {
		s.MaxAccounts = d.MaxAccounts
	}
	if s.TimeoutSeconds < 1 {
		s.TimeoutSeconds = d.TimeoutSeconds
	}
	if s.MaxResponseBytes < 1 {
		s.MaxResponseBytes = d.MaxResponseBytes
	}
	return s
}

// SSOBundle is the sealed secret of an aws_sso account: the OIDC client registration and tokens.
type SSOBundle struct {
	ClientID        string `json:"client_id"`
	ClientSecret    string `json:"client_secret"`
	ClientExpiresAt int64  `json:"client_expires_at,omitempty"`
	AccessToken     string `json:"access_token,omitempty"`
	AccessExpiresAt int64  `json:"access_expires_at,omitempty"`
	RefreshToken    string `json:"refresh_token,omitempty"`
	NeedsRelogin    bool   `json:"needs_relogin,omitempty"`
	UpdatedAt       int64  `json:"updated_at,omitempty"` // unix milliseconds, newest bundle wins
}

// SSODevice is what StartDeviceAuthorization returns for the person to approve.
type SSODevice struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// SSOAccount and SSORole are the portal listings.
type SSOAccount struct {
	AccountID   string `json:"account_id"`
	AccountName string `json:"account_name"`
	Email       string `json:"email,omitempty"`
}

// AWSKeys is a short lived key set from the portal.
type AWSKeys struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	Expires         time.Time
}

// SSOHTTPError carries the status and the AWS error code, never the response text.
type SSOHTTPError struct {
	Status int
	Code   string
}

func (e *SSOHTTPError) Error() string {
	return fmt.Sprintf("cloudconn: aws sso call failed (%d %s)", e.Status, e.Code)
}

// SSOClient speaks plain HTTP to the SSO OIDC and portal endpoints; no SigV4 is involved.
type SSOClient struct {
	HTTP *http.Client
	// OIDCBase and PortalBase build the endpoint per region; tests point them at a fake.
	OIDCBase   func(region string) string
	PortalBase func(region string) string
}

// NewSSOClient returns the production client.
func NewSSOClient() *SSOClient {
	return &SSOClient{
		HTTP:       &http.Client{},
		OIDCBase:   func(r string) string { return "https://oidc." + r + ".amazonaws.com" },
		PortalBase: func(r string) string { return "https://portal.sso." + r + ".amazonaws.com" },
	}
}

// ValidateSSOStart checks the inputs of a login before any call leaves the host.
func ValidateSSOStart(startURL, region string) error {
	u, err := url.Parse(startURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return invalid("start_url must be an https IAM Identity Center start URL")
	}
	if !awsRegionRe.MatchString(region) {
		return invalid("sso_region must be an AWS region such as eu-west-1")
	}
	return nil
}

func validateSSOConfig(cfg Config) error {
	if err := ValidateSSOStart(cfg.StartURL, cfg.SSORegion); err != nil {
		return err
	}
	if !awsAccountRe.MatchString(cfg.AccountID) {
		return invalid("account_id must be a 12 digit AWS account id for aws_sso")
	}
	if !ssoRoleRe.MatchString(cfg.RoleName) {
		return invalid("role_name must be an IAM Identity Center permission set role name")
	}
	return nil
}

func (c *SSOClient) do(ctx context.Context, method, endpoint string, hdr map[string]string, in, out any) error {
	set := SSOCurrentSettings()
	ctx, cancel := context.WithTimeout(ctx, time.Duration(set.TimeoutSeconds)*time.Second)
	defer cancel()
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return errors.New("cloudconn: aws sso endpoint unreachable")
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, int64(set.MaxResponseBytes)))
	if resp.StatusCode/100 != 2 {
		var e struct {
			Error string `json:"error"`
			Type  string `json:"__type"`
			Code  string `json:"code"`
		}
		_ = json.Unmarshal(raw, &e)
		code := e.Error
		if code == "" {
			code = e.Type
		}
		if code == "" {
			code = e.Code
		}
		// The portal answers with the error name in a header and a body of only a message.
		if code == "" {
			code = errorTypeHeader(resp.Header.Get("X-Amzn-Errortype"))
		}
		return &SSOHTTPError{Status: resp.StatusCode, Code: code}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return errors.New("cloudconn: aws sso response was not valid")
	}
	return nil
}

// errorTypeHeader reduces "UnauthorizedException:http://..." to the bare exception name.
func errorTypeHeader(v string) string {
	if i := strings.IndexAny(v, ":"); i >= 0 {
		v = v[:i]
	}
	if i := strings.LastIndex(v, "#"); i >= 0 {
		v = v[i+1:]
	}
	return strings.TrimSpace(v)
}

// Register creates the public OIDC client for one start URL; refresh needs the issuer URL.
func (c *SSOClient) Register(ctx context.Context, region, startURL, clientName string) (SSOBundle, error) {
	var out struct {
		ClientID     string `json:"clientId"`
		ClientSecret string `json:"clientSecret"`
		ExpiresAt    int64  `json:"clientSecretExpiresAt"`
	}
	in := map[string]any{"clientName": clientName, "clientType": "public", "scopes": []string{ssoScope},
		"grantTypes": []string{ssoGrantDevice, ssoGrantRefresh}, "issuerUrl": startURL}
	if err := c.do(ctx, http.MethodPost, c.OIDCBase(region)+"/client/register", nil, in, &out); err != nil {
		return SSOBundle{}, err
	}
	if out.ClientID == "" || out.ClientSecret == "" {
		return SSOBundle{}, errors.New("cloudconn: aws sso registration returned no client")
	}
	secretmask.Add(out.ClientSecret)
	return SSOBundle{ClientID: out.ClientID, ClientSecret: out.ClientSecret, ClientExpiresAt: out.ExpiresAt}, nil
}

// StartDevice begins the device authorization for a registered client.
func (c *SSOClient) StartDevice(ctx context.Context, region, startURL string, b SSOBundle) (SSODevice, error) {
	var out struct {
		DeviceCode  string `json:"deviceCode"`
		UserCode    string `json:"userCode"`
		URI         string `json:"verificationUri"`
		URIComplete string `json:"verificationUriComplete"`
		ExpiresIn   int    `json:"expiresIn"`
		Interval    int    `json:"interval"`
	}
	in := map[string]string{"clientId": b.ClientID, "clientSecret": b.ClientSecret, "startUrl": startURL}
	if err := c.do(ctx, http.MethodPost, c.OIDCBase(region)+"/device_authorization", nil, in, &out); err != nil {
		return SSODevice{}, err
	}
	secretmask.Add(out.DeviceCode)
	if out.Interval <= 0 {
		out.Interval = 5
	}
	return SSODevice{DeviceCode: out.DeviceCode, UserCode: out.UserCode, VerificationURI: out.URI, VerificationURIComplete: out.URIComplete, ExpiresIn: out.ExpiresIn, Interval: out.Interval}, nil
}

type ssoTokenResp struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresIn    int64  `json:"expiresIn"`
}

func (c *SSOClient) token(ctx context.Context, region string, in map[string]string, b SSOBundle, now time.Time) (SSOBundle, error) {
	in["clientId"], in["clientSecret"] = b.ClientID, b.ClientSecret
	var out ssoTokenResp
	if err := c.do(ctx, http.MethodPost, c.OIDCBase(region)+"/token", nil, in, &out); err != nil {
		return b, err
	}
	if out.AccessToken == "" {
		return b, errors.New("cloudconn: aws sso returned no access token")
	}
	secretmask.Add(out.AccessToken)
	secretmask.Add(out.RefreshToken)
	b.AccessToken, b.AccessExpiresAt, b.NeedsRelogin = out.AccessToken, now.Unix()+out.ExpiresIn, false
	if out.RefreshToken != "" {
		b.RefreshToken = out.RefreshToken
	}
	b.UpdatedAt = now.UnixMilli()
	return b, nil
}

// DeviceToken polls the device grant; pending and slow_down come back as sentinel errors.
func (c *SSOClient) DeviceToken(ctx context.Context, region string, b SSOBundle, deviceCode string, now time.Time) (SSOBundle, error) {
	out, err := c.token(ctx, region, map[string]string{"grantType": ssoGrantDevice, "deviceCode": deviceCode}, b, now)
	var he *SSOHTTPError
	if errors.As(err, &he) {
		switch he.Code {
		case "authorization_pending":
			return b, ErrSSOPending
		case "slow_down":
			return b, ErrSSOSlowDown
		case "expired_token", "access_denied", "invalid_grant":
			return b, ErrSSODenied
		}
	}
	return out, err
}

// Refresh exchanges the refresh token for a new access token.
func (c *SSOClient) Refresh(ctx context.Context, region string, b SSOBundle, now time.Time) (SSOBundle, error) {
	return c.token(ctx, region, map[string]string{"grantType": ssoGrantRefresh, "refreshToken": b.RefreshToken}, b, now)
}

func (c *SSOClient) portalGet(ctx context.Context, region, path, token string, q url.Values, out any) error {
	return c.do(ctx, http.MethodGet, c.PortalBase(region)+path+"?"+q.Encode(), map[string]string{"x-amz-sso_bearer_token": token}, nil, out)
}

// ListAccounts pages through the accounts the signed in user may use, up to max.
func (c *SSOClient) ListAccounts(ctx context.Context, region, token string, limit int) ([]SSOAccount, error) {
	var all []SSOAccount
	next := ""
	for len(all) < limit {
		q := url.Values{"max_result": {"100"}}
		if next != "" {
			q.Set("next_token", next)
		}
		var out struct {
			List []struct {
				ID    string `json:"accountId"`
				Name  string `json:"accountName"`
				Email string `json:"emailAddress"`
			} `json:"accountList"`
			Next string `json:"nextToken"`
		}
		if err := c.portalGet(ctx, region, "/assignment/accounts", token, q, &out); err != nil {
			return nil, err
		}
		for _, a := range out.List {
			all = append(all, SSOAccount{AccountID: a.ID, AccountName: a.Name, Email: a.Email})
		}
		if out.Next == "" {
			break
		}
		next = out.Next
	}
	if len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

// ListRoles returns the role names the user holds in one account.
func (c *SSOClient) ListRoles(ctx context.Context, region, token, accountID string) ([]string, error) {
	var names []string
	next := ""
	for {
		q := url.Values{"account_id": {accountID}, "max_result": {"100"}}
		if next != "" {
			q.Set("next_token", next)
		}
		var out struct {
			List []struct {
				RoleName string `json:"roleName"`
			} `json:"roleList"`
			Next string `json:"nextToken"`
		}
		if err := c.portalGet(ctx, region, "/assignment/roles", token, q, &out); err != nil {
			return nil, err
		}
		for _, r := range out.List {
			names = append(names, r.RoleName)
		}
		if out.Next == "" {
			return names, nil
		}
		next = out.Next
	}
}

// RoleCredentials trades the access token for short lived keys of one account role.
func (c *SSOClient) RoleCredentials(ctx context.Context, region, token, accountID, role string) (AWSKeys, error) {
	var out struct {
		Creds struct {
			AccessKeyID string `json:"accessKeyId"`
			Secret      string `json:"secretAccessKey"`
			Session     string `json:"sessionToken"`
			Expiration  int64  `json:"expiration"`
		} `json:"roleCredentials"`
	}
	q := url.Values{"account_id": {accountID}, "role_name": {role}}
	if err := c.portalGet(ctx, region, "/federation/credentials", token, q, &out); err != nil {
		return AWSKeys{}, err
	}
	if out.Creds.AccessKeyID == "" || out.Creds.Secret == "" {
		return AWSKeys{}, errors.New("cloudconn: aws sso returned no role credentials")
	}
	secretmask.Add(out.Creds.Secret)
	secretmask.Add(out.Creds.Session)
	return AWSKeys{AccessKeyID: out.Creds.AccessKeyID, SecretAccessKey: out.Creds.Secret, SessionToken: out.Creds.Session, Expires: time.UnixMilli(out.Creds.Expiration)}, nil
}

// SSONeedsRelogin reports from the sealed bundle alone whether a person must sign in again.
func SSONeedsRelogin(raw []byte, now time.Time) bool {
	var b SSOBundle
	if json.Unmarshal(raw, &b) != nil {
		return true
	}
	if b.NeedsRelogin {
		return true
	}
	if now.Unix() < b.AccessExpiresAt {
		return false
	}
	return b.RefreshToken == "" || (b.ClientExpiresAt > 0 && now.Unix() >= b.ClientExpiresAt)
}

type ssoEntry struct {
	mu      sync.Mutex // single flight: one refresh or credential read per account at a time
	keys    AWSKeys
	renewAt time.Time
	bundle  SSOBundle
	have    bool
}

// SSOBroker keeps role credentials per account, refreshes the access token lazily and records
// a needs_relogin state when the refresh path ends.
type SSOBroker struct {
	Client *SSOClient
	now    func() time.Time

	mu      sync.Mutex
	entries map[string]*ssoEntry
	persist func(ctx context.Context, account string, bundle []byte) error
}

// NewSSOBroker returns a broker over the given client; nil means the production client.
func NewSSOBroker(c *SSOClient) *SSOBroker {
	if c == nil {
		c = NewSSOClient()
	}
	return &SSOBroker{Client: c, now: time.Now, entries: map[string]*ssoEntry{}}
}

// SetPersist registers where refreshed bundles are sealed again.
func (b *SSOBroker) SetPersist(fn func(ctx context.Context, account string, bundle []byte) error) {
	b.mu.Lock()
	b.persist = fn
	b.mu.Unlock()
}

func (b *SSOBroker) entry(name string) *ssoEntry {
	b.mu.Lock()
	defer b.mu.Unlock()
	e := b.entries[name]
	if e == nil {
		e = &ssoEntry{}
		b.entries[name] = e
	}
	return e
}

// Forget drops cached keys for an account, for delete and disable.
func (b *SSOBroker) Forget(name string) {
	b.mu.Lock()
	delete(b.entries, name)
	b.mu.Unlock()
}

func ssoSkew() time.Duration {
	return time.Duration(SSOCurrentSettings().TokenSkewSeconds) * time.Second
}

func ssoRenewPercent() int { return SSOCurrentSettings().RenewPercent }

func (b *SSOBroker) save(ctx context.Context, name string, e *ssoEntry, nb SSOBundle) {
	e.bundle = nb
	b.mu.Lock()
	fn := b.persist
	b.mu.Unlock()
	if fn == nil {
		return
	}
	if raw, err := json.Marshal(nb); err == nil { //nolint:gosec // sealed bundle, never logged
		_ = fn(ctx, name, raw)
	}
}

// refresh swaps the refresh token for a new access token; an ended session is recorded, not retried.
func (b *SSOBroker) refresh(ctx context.Context, c Connection, e *ssoEntry, cur SSOBundle) (SSOBundle, error) {
	now := b.now()
	if cur.RefreshToken != "" && (cur.ClientExpiresAt == 0 || now.Unix() < cur.ClientExpiresAt) {
		nb, err := b.Client.Refresh(ctx, c.Config.SSORegion, cur, now)
		if err == nil {
			b.save(ctx, c.Name, e, nb)
			return nb, nil
		}
		var he *SSOHTTPError
		if !errors.As(err, &he) || he.Status >= 500 || he.Status == http.StatusTooManyRequests {
			return cur, err
		}
	}
	cur.NeedsRelogin, cur.UpdatedAt = true, now.UnixMilli()
	b.save(ctx, c.Name, e, cur)
	return cur, ErrSSONeedsRelogin
}

// Credentials returns role keys for the account, reading new ones only past the renewal point.
func (b *SSOBroker) Credentials(ctx context.Context, c Connection, raw []byte) (AWSKeys, error) {
	e := b.entry(c.Name)
	e.mu.Lock()
	defer e.mu.Unlock()
	now := b.now()
	if e.have && now.Before(e.renewAt) {
		return e.keys, nil
	}
	var cur SSOBundle
	if err := json.Unmarshal(raw, &cur); err != nil || cur.ClientID == "" {
		return AWSKeys{}, errors.New("cloudconn: aws sso bundle is not valid")
	}
	if e.bundle.UpdatedAt > cur.UpdatedAt {
		cur = e.bundle
	}
	if cur.NeedsRelogin {
		return AWSKeys{}, ErrSSONeedsRelogin
	}
	var err error
	refreshed := false
	if now.Unix() >= cur.AccessExpiresAt-int64(ssoSkew().Seconds()) {
		if cur, err = b.refresh(ctx, c, e, cur); err != nil {
			return AWSKeys{}, err
		}
		refreshed = true
	}
	keys, err := b.Client.RoleCredentials(ctx, c.Config.SSORegion, cur.AccessToken, c.Config.AccountID, c.Config.RoleName)
	var he *SSOHTTPError
	if errors.As(err, &he) && he.Status == http.StatusUnauthorized && !refreshed {
		if cur, err = b.refresh(ctx, c, e, cur); err != nil {
			return AWSKeys{}, err
		}
		keys, err = b.Client.RoleCredentials(ctx, c.Config.SSORegion, cur.AccessToken, c.Config.AccountID, c.Config.RoleName)
	}
	if err != nil {
		return AWSKeys{}, err
	}
	e.keys, e.have = keys, true
	e.renewAt = now.Add(keys.Expires.Sub(now) * time.Duration(ssoRenewPercent()) / 100)
	return keys, nil
}

// classifySSO maps a broker failure to a health class without carrying provider text.
func classifySSO(err error) string {
	var he *SSOHTTPError
	switch {
	case errors.Is(err, ErrSSONeedsRelogin):
		return ClassNeedsRelogin
	case errors.As(err, &he) && (he.Status == http.StatusUnauthorized || he.Status == http.StatusForbidden):
		return ClassAuthFailed
	case errors.As(err, &he):
		return ClassUnknown
	case strings.Contains(err.Error(), "unreachable"), errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return ClassUnreachable
	}
	return ClassCredentialUnresolved
}

var (
	sharedSSOOnce   sync.Once
	sharedSSOBroker *SSOBroker
)

// DefaultSSOBroker is shared so every provider instance reuses one credential cache.
func DefaultSSOBroker() *SSOBroker {
	sharedSSOOnce.Do(func() { sharedSSOBroker = NewSSOBroker(nil) })
	return sharedSSOBroker
}

// SSOBroker returns the broker this provider uses.
func (p *AWSProvider) SSOBroker() *SSOBroker {
	if p.opts.SSO != nil {
		return p.opts.SSO
	}
	return DefaultSSOBroker()
}

// SSOKeys returns role keys for an aws_sso target, for callers that need raw keys.
func (p *AWSProvider) SSOKeys(ctx context.Context, t Target) (AWSKeys, string, error) {
	if t.Connection.AuthMethod != AuthAWSSSO || t.Credential.Len() == 0 {
		return AWSKeys{}, ClassCredentialUnresolved, errors.New("cloudconn: no sso login for this account")
	}
	k, err := p.SSOBroker().Credentials(ctx, t.Connection, t.Credential.Reveal())
	if err != nil {
		if errors.Is(err, ErrSSONeedsRelogin) {
			return AWSKeys{}, ClassNeedsRelogin, err
		}
		return AWSKeys{}, classifySSO(err), errors.New("cloudconn: aws sso credentials could not be read")
	}
	return k, ClassOK, nil
}

// ssoProvider hands the SDK keys from the broker on every retrieval; the broker decides when to renew.
func (p *AWSProvider) ssoProvider(t Target) aws.CredentialsProvider {
	return aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
		k, _, err := p.SSOKeys(ctx, t)
		if err != nil {
			return aws.Credentials{}, err
		}
		return aws.Credentials{AccessKeyID: k.AccessKeyID, SecretAccessKey: k.SecretAccessKey, SessionToken: k.SessionToken, Source: "levelrail-aws-sso"}, nil
	})
}
