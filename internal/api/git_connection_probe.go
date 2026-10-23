package api

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/gitprovider"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	probeOK          = "ok"
	probeBadURL      = "bad_url"
	probeBadToken    = "bad_token"
	probeBadScope    = "bad_scope"
	probeTLS         = "tls"
	probeUnreachable = "unreachable"

	gitProbeTimeout = 10 * time.Second
)

type gitProbeRequest struct {
	Provider    string `json:"provider"`
	InstanceURL string `json:"instance_url"`
	Token       string `json:"token"`
}

// gitProbeResult is a typed outcome: Kind is stable for the UI and CLI, Fix says what to do next.
type gitProbeResult struct {
	OK      bool   `json:"ok"`
	Kind    string `json:"kind"`
	Message string `json:"message"`
	Fix     string `json:"fix,omitempty"`
	Account string `json:"account,omitempty"`
	// Token reports the scopes and repository access of a pasted token.
	Token *gitTokenDetails `json:"token,omitempty"`
}

// gitProbeVersionPath is the unauthenticated endpoint that proves the URL is the right kind of server.
var gitProbeVersionPath = map[string]string{
	gitProviderGitea:           "/api/v1/version",
	gitProviderForgejo:         "/api/v1/version",
	gitProviderGitLab:          "/api/v4/version",
	gitProviderBitbucketServer: "/rest/api/1.0/application-properties",
}

const (
	fixNotThisServer = "Check the URL points at the root of the instance, not a repository or a sub-path."
	fixRetryAccess   = "Create a new personal access token with repository read access and paste it again."
)

// handleTestGitConnection handles POST /api/v1/git-connections/test: checks an instance URL and optional token without saving anything.
func (rt *Router) handleTestGitConnection(w http.ResponseWriter, r *http.Request) {
	var req gitProbeRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	adapter, ok := rt.gitAdapter(strings.TrimSpace(req.Provider))
	if !ok {
		writeError(w, http.StatusBadRequest, errGitProviderOneOf)
		return
	}
	writeJSON(w, http.StatusOK, rt.probeGitInstance(r.Context(), adapter, req))
}

func (rt *Router) probeGitInstance(ctx context.Context, adapter gitProviderAdapter, req gitProbeRequest) gitProbeResult {
	base := strings.TrimRight(strings.TrimSpace(req.InstanceURL), "/")
	token := strings.TrimSpace(req.Token)
	if adapter.provider == gitProviderBitbucket {
		base = "https://bitbucket.org"
	}
	if u, err := url.Parse(base); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return gitProbeResult{Kind: probeBadURL, Message: "The instance URL is not a valid http or https address.", Fix: "Enter the full address of your " + adapter.label + " instance, for example https://git.example.com."}
	}
	if token == "" {
		return probeReachable(ctx, adapter, base)
	}
	conn := store.GitConnection{Provider: adapter.provider, InstanceURL: base, Host: store.RepoHost(base)}
	acct, err := rt.lookupGitAccount(ctx, adapter, conn, token)
	if err != nil {
		return classifyGitProbeError(adapter.label, err)
	}
	res := gitProbeResult{OK: true, Kind: probeOK, Message: "Connected to " + adapter.label + ".", Account: acct.Login}
	if adapter.patAuth {
		return res
	}
	details, derr := rt.inspectGitToken(ctx, adapter.provider, conn, token)
	if derr != nil {
		return gitProbeResult{Kind: probeBadToken, Message: adapter.label + " accepted the token but would not list repositories.", Fix: fixRetryAccess, Account: acct.Login}
	}
	res.Token = &details
	if len(details.Missing) > 0 {
		res.OK, res.Kind = false, probeBadScope
		res.Message, res.Fix = "The token is missing scopes: "+strings.Join(details.Missing, ", ")+".", fixRetryAccess
	}
	return res
}

func probeReachable(ctx context.Context, adapter gitProviderAdapter, base string) gitProbeResult {
	pctx, cancel := context.WithTimeout(ctx, gitProbeTimeout)
	defer cancel()
	hreq, err := http.NewRequestWithContext(pctx, http.MethodGet, base+gitProbeVersionPath[adapter.provider], nil)
	if err != nil {
		return gitProbeResult{Kind: probeBadURL, Message: "The instance URL is not a valid address.", Fix: "Check the URL for typos."}
	}
	resp, err := gitprovider.NewGuardedClient().Do(hreq)
	if err != nil {
		return classifyGitProbeError(adapter.label, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return gitProbeResult{Kind: probeBadURL, Message: "The server answered, but it does not look like " + adapter.label + ".", Fix: fixNotThisServer}
	}
	return gitProbeResult{OK: true, Kind: probeOK, Message: adapter.label + " is reachable. Add a token to also verify access."}
}

func classifyGitProbeError(label string, err error) gitProbeResult {
	var apiErr *gitprovider.APIError
	var unknownCA x509.UnknownAuthorityError
	var badCert x509.CertificateInvalidError
	var badHost x509.HostnameError
	var netErr net.Error
	switch {
	case errors.As(err, &unknownCA), errors.As(err, &badCert), errors.As(err, &badHost),
		strings.Contains(err.Error(), "x509:"), strings.Contains(err.Error(), "tls:"):
		return gitProbeResult{Kind: probeTLS, Message: "The TLS certificate of the server was rejected.", Fix: "Install a trusted certificate on the server, or add your private CA to this control plane's trust store."}
	case strings.Contains(err.Error(), "netguard:"):
		return gitProbeResult{Kind: probeUnreachable, Message: "The server is on an internal address this control plane blocks.", Fix: "Set " + gitprovider.AllowPrivateEnv + "=true on the control plane to allow a self-hosted server on a private network."}
	case errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden || apiErr.StatusCode == http.StatusOK):
		return gitProbeResult{Kind: probeBadToken, Message: label + " rejected the token.", Fix: fixRetryAccess}
	case errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound:
		return gitProbeResult{Kind: probeBadURL, Message: "The server answered, but it does not look like " + label + ".", Fix: fixNotThisServer}
	case errors.As(err, &netErr), errors.Is(err, context.DeadlineExceeded):
		return gitProbeResult{Kind: probeUnreachable, Message: "The control plane could not reach the server.", Fix: "Check the hostname, the port, and that this machine's network or firewall allows outbound access to it."}
	}
	return gitProbeResult{Kind: probeUnreachable, Message: label + " did not give a usable answer.", Fix: "Check the URL and token, then try again."}
}
