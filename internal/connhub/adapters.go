package connhub

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/cloudconn"
	"github.com/GLINCKER/levelrail/internal/registryvendor"
	"github.com/GLINCKER/levelrail/internal/store"
)

// maxDetail bounds a stored error text shown as health detail.
const maxDetail = 200

type adapter struct {
	kind string
	list func(ctx context.Context) ([]Connection, error)
}

func (s *Service) adapters() []adapter {
	return []adapter{
		{KindGit, s.listGit},
		{KindRegistry, s.listRegistry},
		{KindNotify, s.listNotify},
		{KindSecret, s.listSecrets},
		{KindDNS, s.listDNS},
		{KindCloud, s.listCloud},
		{KindWebhook, s.listWebhooks},
	}
}

func clip(s string) string {
	if len(s) > maxDetail {
		return s[:maxDetail]
	}
	return s
}

func parseTime(raw string) *time.Time {
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return nil
	}
	return ptrTime(t)
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Host
}

// gitHealth maps the git_connection_status row, keyed by connection id (or "github").
func gitHealth(ctx context.Context, r GitReader, key string) (health, detail string, checked *time.Time, err error) {
	st, ok, err := r.GetGitConnectionStatus(ctx, key)
	if err != nil || !ok {
		return HealthUnknown, "", nil, err
	}
	checked = ptrTime(st.UpdatedAt)
	switch st.Status {
	case store.GitConnOK:
		return HealthOK, "", checked, nil
	case store.GitConnNeedsReauthorize, store.GitConnRevoked:
		return HealthNeedsReconnect, clip(st.Detail), checked, nil
	case store.GitConnRateLimited:
		return HealthDegraded, clip(st.Detail), checked, nil
	}
	return HealthUnknown, clip(st.Detail), checked, nil
}

func gitCaps(provider string) []string {
	switch provider {
	case "github":
		return []string{CapSource, CapWebhook, CapStatus, CapDeployments, CapPreviews}
	case store.GitProviderGitLab, store.GitProviderBitbucket, store.GitProviderGitea, store.GitProviderForgejo:
		return []string{CapSource, CapWebhook, CapStatus, CapPreviews}
	}
	return []string{CapSource, CapWebhook}
}

func (s *Service) listGit(ctx context.Context) ([]Connection, error) {
	r := s.src.Git
	if r == nil {
		return nil, nil
	}
	out := []Connection{}
	gh, err := r.GetGitHubAppConnection(ctx)
	if err != nil && !errors.Is(err, store.ErrGitHubAppConnectionNotFound) {
		return nil, err
	}
	if err == nil {
		health, detail, checked, herr := gitHealth(ctx, r, "github")
		if herr != nil {
			return nil, herr
		}
		base := Connection{
			Kind: KindGit, Provider: "github", Host: hostOf(gh.InstanceURL), AuthMethod: AuthAppInstall,
			Health: health, HealthDetail: detail, CheckedAt: checked, Capabilities: gitCaps("github"), CreatedAt: parseTime(gh.CreatedAt),
		}
		installs, ierr := r.ListGitHubAppInstallations(ctx)
		if ierr != nil {
			return nil, ierr
		}
		for _, in := range installs {
			c := base
			c.Ref = "github:install:" + strconv.FormatInt(in.InstallationID, 10)
			c.Label, c.Account = in.AccountLogin, in.AccountLogin
			out = append(out, c)
		}
		if len(installs) == 0 {
			c := base
			c.Ref = "github:app:" + strconv.FormatInt(gh.AppID, 10)
			c.Label = "GitHub App"
			if gh.Slug != nil {
				c.Label = *gh.Slug
			}
			out = append(out, c)
		}
	}
	conns, err := r.ListGitConnections(ctx, "")
	if err != nil {
		return nil, err
	}
	for _, g := range conns {
		health, detail, checked, herr := gitHealth(ctx, r, g.ID)
		if herr != nil {
			return nil, herr
		}
		bound, berr := r.ListGitSourcesByConnection(ctx, g.ID)
		if berr != nil {
			return nil, berr
		}
		auth := AuthOAuth
		if g.Provider == store.GitProviderBitbucketServer || g.Provider == store.GitProviderAzureDevOps {
			auth = AuthToken
		}
		host := g.Host
		if host == "" {
			host = hostOf(g.InstanceURL)
		}
		out = append(out, Connection{
			Ref: "git:" + g.ID, Kind: KindGit, Provider: g.Provider, Label: g.Label, Account: g.AccountName, Host: host,
			AuthMethod: auth, Health: health, HealthDetail: detail, CheckedAt: checked, Capabilities: gitCaps(g.Provider),
			BoundCount: len(bound), CreatedAt: parseTime(g.CreatedAt), IsDefault: g.IsDefault,
		})
	}
	return out, nil
}

func (s *Service) listRegistry(ctx context.Context) ([]Connection, error) {
	if s.src.Registry == nil {
		return nil, nil
	}
	creds, err := s.src.Registry.ListRegistryCredentials(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Connection, 0, len(creds))
	for _, c := range creds {
		conn := Connection{
			Ref: "registry:" + c.ID, Kind: KindRegistry, Provider: registryProvider(c), Label: c.Name, Account: c.Username, Host: c.RegistryHost,
			AuthMethod: AuthToken, Health: HealthUnknown, ExpiresAt: ptrTime(derefTime(c.ExpiresAt)),
			Capabilities: []string{CapRegistryPull}, CreatedAt: parseTime(c.CreatedAt),
		}
		if c.Preset != "" {
			conn.AuthMethod = AuthWorkloadIdentity
		}
		switch {
		case c.LastRefreshError == registryvendor.DockerHubRejected:
			conn.Health, conn.HealthDetail = HealthNeedsReconnect, c.LastRefreshError
		case c.LastRefreshError != "":
			conn.Health, conn.HealthDetail = HealthDegraded, clip(c.LastRefreshError)
		case c.LastRefreshedAt != nil:
			conn.Health, conn.CheckedAt = HealthOK, ptrTime(*c.LastRefreshedAt)
		}
		out = append(out, conn)
	}
	return out, nil
}

func registryProvider(c store.RegistryCredential) string {
	if c.Preset != "" {
		return c.Preset
	}
	if registryvendor.IsDockerHubHost(c.RegistryHost) {
		return "dockerhub"
	}
	return c.RegistryHost
}

func derefTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

func (s *Service) listNotify(ctx context.Context) ([]Connection, error) {
	if s.src.Notify == nil {
		return nil, nil
	}
	chans, err := s.src.Notify.ListNotificationChannels(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Connection, 0, len(chans))
	for _, c := range chans {
		conn := Connection{
			Ref: "notify:" + c.ID, Kind: KindNotify, Provider: c.Kind, Label: c.Name, AuthMethod: AuthURLSecret,
			Health: HealthUnknown, Capabilities: []string{CapNotify}, CreatedAt: parseTime(c.CreatedAt),
		}
		if !c.Enabled {
			conn.HealthDetail = "disabled"
		}
		out = append(out, conn)
	}
	return out, nil
}

func (s *Service) listSecrets(ctx context.Context) ([]Connection, error) {
	if s.src.Secrets == nil {
		return nil, nil
	}
	conns, err := s.src.Secrets.ListSecretConnections(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Connection, 0, len(conns))
	for _, c := range conns {
		created := c.CreatedAt
		out = append(out, Connection{
			Ref: "secret:" + c.Name, Kind: KindSecret, Provider: c.Type, Label: c.Name, AuthMethod: AuthToken,
			Health: HealthUnknown, Capabilities: []string{CapSecrets}, CreatedAt: ptrTime(created),
		})
	}
	return out, nil
}

func (s *Service) listDNS(ctx context.Context) ([]Connection, error) {
	r := s.src.DNS
	if r == nil {
		return nil, nil
	}
	out := []Connection{}
	add := func(provider, label string) {
		out = append(out, Connection{
			Ref: "dns:" + provider, Kind: KindDNS, Provider: provider, Label: label, AuthMethod: AuthToken,
			Health: HealthUnknown, Capabilities: []string{CapDNS},
		})
	}
	cf, err := r.GetCloudflareDNSSettings(ctx)
	if err != nil {
		return nil, err
	}
	if cf.Enabled {
		add("cloudflare", "Cloudflare DNS")
	}
	r53, err := r.GetRoute53DNSSettings(ctx)
	if err != nil {
		return nil, err
	}
	if r53.Enabled {
		add("route53", "Route 53")
	}
	g, err := r.GetGoogleCloudDNSSettings(ctx)
	if err != nil {
		return nil, err
	}
	if g.Enabled {
		add("google", "Google Cloud DNS")
	}
	return out, nil
}

func (s *Service) listCloud(ctx context.Context) ([]Connection, error) {
	if s.src.Cloud == nil {
		return nil, nil
	}
	conns, err := s.src.Cloud.ListCloudConnections(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Connection, 0, len(conns))
	for _, c := range conns {
		conn := Connection{
			Ref: "cloud:" + c.Name, Kind: KindCloud, Provider: c.Provider, Label: c.Name, AuthMethod: cloudAuth(c.AuthMethod),
			Health: HealthUnknown, CheckedAt: ptrTime(c.LastCheckedAt), Capabilities: []string{CapProvisionHost},
			CreatedAt: ptrTime(c.CreatedAt),
		}
		switch c.LastCheckClass {
		case "":
		case cloudconn.ClassOK:
			conn.Health = HealthOK
		case cloudconn.ClassAuthFailed, cloudconn.ClassCredentialUnresolved:
			conn.Health, conn.HealthDetail = HealthNeedsReconnect, c.LastCheckClass
		case cloudconn.ClassUnknown, cloudconn.ClassUnsupported:
			conn.HealthDetail = c.LastCheckClass
		default:
			conn.Health, conn.HealthDetail = HealthDegraded, c.LastCheckClass
		}
		out = append(out, conn)
	}
	return out, nil
}

func cloudAuth(method string) string {
	if method == cloudconn.AuthOIDCFederation || method == cloudconn.AuthRoleAssumption {
		return AuthWorkloadIdentity
	}
	return AuthToken
}

func (s *Service) listWebhooks(ctx context.Context) ([]Connection, error) {
	if s.src.Webhooks == nil {
		return nil, nil
	}
	out := []Connection{}
	events, err := s.src.Webhooks.ListEventWebhooks(ctx)
	if err != nil {
		return nil, err
	}
	for _, w := range events {
		c := Connection{
			Ref: "webhook:event:" + w.ID, Kind: KindWebhook, Provider: "event", Label: w.Name, AuthMethod: AuthURLSecret,
			Health: HealthUnknown, Capabilities: []string{CapWebhook}, CreatedAt: parseTime(w.CreatedAt),
		}
		if !w.Enabled {
			c.HealthDetail = "disabled"
		}
		out = append(out, c)
	}
	status, err := s.src.Webhooks.ListStatusWebhooks(ctx)
	if err != nil {
		return nil, err
	}
	for _, w := range status {
		c := Connection{
			Ref: "webhook:status:" + w.ID, Kind: KindWebhook, Provider: "status", Label: w.Label, AuthMethod: AuthURLSecret,
			Health: HealthOK, Capabilities: []string{CapWebhook}, CreatedAt: ptrTime(w.CreatedAt),
		}
		switch {
		case w.Paused:
			c.Health, c.HealthDetail = HealthDegraded, clip("paused: "+w.PausedReason)
		case w.ConsecutiveFailures > 0:
			c.Health, c.HealthDetail = HealthDegraded, strconv.Itoa(w.ConsecutiveFailures)+" consecutive delivery failures"
		}
		out = append(out, c)
	}
	return out, nil
}

// legacyAppBindings derives links from columns that stay authoritative. Only the git source link
// is derived today; a source with no connection id relied on the GitHub App and has no ref to show.
func (s *Service) legacyAppBindings(ctx context.Context, app string) ([]Binding, error) {
	if s.src.Git == nil {
		return nil, nil
	}
	src, err := s.src.Git.GetGitSource(ctx, app)
	if errors.Is(err, store.ErrGitSourceNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if src == nil || src.ConnectionID == "" {
		return nil, nil
	}
	return []Binding{{Ref: "git:" + src.ConnectionID, TargetType: "app", TargetID: app, Role: "source", Origin: "legacy"}}, nil
}
