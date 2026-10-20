package models

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Diagnosis codes a probe can return; the dashboard maps each to a typed fix.
const (
	DiagRefused         = "connection_refused"
	DiagTimeout         = "timeout"
	DiagDNS             = "dns"
	DiagBlocked         = "address_blocked"
	DiagPrivateDisabled = "private_not_enabled"
	DiagAuth            = "auth_failed"
	DiagWrongPort       = "wrong_port"
	DiagNotFound        = "route_not_found"
	DiagBadResponse     = "bad_response"
	DiagTLS             = "tls"
	DiagNoModels        = "no_models"
	DiagUpstream        = "upstream_error"
)

// EndpointProbe is the outcome of one list-models call against an endpoint.
type EndpointProbe struct {
	State   string
	Latency time.Duration
	Models  []string
	// Code is empty on success; Message is operator-safe text without secrets.
	Code    string
	Message string
	Status  int
}

// EndpointChecker calls an endpoint's list-models route.
type EndpointChecker interface {
	Probe(ctx context.Context, baseURL, apiType, credential string) EndpointProbe
}

// EndpointProber is the production EndpointChecker, built on the guarded transport.
type EndpointProber struct {
	client *http.Client
	slow   time.Duration
}

// NewEndpointProber builds a prober; slow is the latency above which a
// working endpoint is reported as slow.
func NewEndpointProber(slow time.Duration) *EndpointProber {
	return &EndpointProber{
		client: &http.Client{
			Transport:     newEndpointTransport(5*time.Second, 0),
			Timeout:       10 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		slow: slow,
	}
}

func listPath(apiType string) string {
	if apiType == store.EndpointAPIOllama {
		return "/api/tags"
	}
	return "/v1/models"
}

// Probe implements EndpointChecker.
func (p *EndpointProber) Probe(ctx context.Context, baseURL, apiType, credential string) EndpointProbe {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+listPath(apiType), nil)
	if err != nil {
		return EndpointProbe{State: store.EndpointError, Code: DiagBadResponse, Message: "base URL is not valid"}
	}
	if credential != "" {
		req.Header.Set("Authorization", "Bearer "+credential)
	}
	req.Header.Set("Accept", "application/json")
	start := time.Now()
	resp, err := p.client.Do(req)
	if err != nil {
		return classifyDialError(err, time.Since(start))
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	res := EndpointProbe{Latency: time.Since(start), Status: resp.StatusCode}
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		res.State, res.Code, res.Message = store.EndpointAuthFailed, DiagAuth, "the server rejected the credential"
		return res
	case resp.StatusCode == http.StatusNotFound:
		res.State, res.Code, res.Message = store.EndpointError, DiagNotFound, "the server has no "+listPath(apiType)+" route"
		return res
	case resp.StatusCode >= 500:
		res.State, res.Code, res.Message = store.EndpointError, DiagUpstream, fmt.Sprintf("the server answered HTTP %d", resp.StatusCode)
		return res
	case resp.StatusCode != http.StatusOK:
		res.State, res.Code, res.Message = store.EndpointError, DiagBadResponse, fmt.Sprintf("the server answered HTTP %d", resp.StatusCode)
		return res
	}
	names, perr := parseModelList(apiType, body)
	if perr != nil {
		code := DiagBadResponse
		if strings.HasPrefix(strings.TrimSpace(string(body)), "<") {
			code = DiagWrongPort
		}
		res.State, res.Code, res.Message = store.EndpointError, code, "the response is not a model list"
		return res
	}
	res.Models = names
	res.State = store.EndpointOnline
	if p.slow > 0 && res.Latency > p.slow {
		res.State = store.EndpointSlow
	}
	if len(names) == 0 {
		res.Code, res.Message = DiagNoModels, "the server answered but lists no models"
	}
	return res
}

func parseModelList(apiType string, body []byte) ([]string, error) {
	var names []string
	if apiType == store.EndpointAPIOllama {
		var doc struct {
			Models []struct {
				Name string `json:"name"`
			} `json:"models"`
		}
		if err := json.Unmarshal(body, &doc); err != nil {
			return nil, fmt.Errorf("models: parse ollama tags: %w", err)
		}
		for _, m := range doc.Models {
			names = append(names, m.Name)
		}
	} else {
		var doc struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &doc); err != nil {
			return nil, fmt.Errorf("models: parse model list: %w", err)
		}
		for _, m := range doc.Data {
			names = append(names, m.ID)
		}
	}
	sort.Strings(names)
	return names, nil
}

func classifyDialError(err error, took time.Duration) EndpointProbe {
	res := EndpointProbe{Latency: took, State: store.EndpointUnreachable}
	var dnsErr *net.DNSError
	var ne net.Error
	var certErr *tls.CertificateVerificationError
	switch {
	case errors.Is(err, ErrEndpointBlocked):
		res.State, res.Code, res.Message = store.EndpointError, DiagBlocked, "that address is link-local or a metadata address and is never allowed"
	case errors.Is(err, ErrEndpointPrivate):
		res.State, res.Code, res.Message = store.EndpointError, DiagPrivateDisabled, "internal destinations are not enabled on this control plane"
	case errors.As(err, &dnsErr):
		res.Code, res.Message = DiagDNS, "the host name does not resolve"
	case errors.As(err, &certErr):
		res.State, res.Code, res.Message = store.EndpointError, DiagTLS, "the TLS certificate is not trusted"
	case errors.Is(err, syscall.ECONNREFUSED):
		res.Code, res.Message = DiagRefused, "nothing is listening on that address and port"
	case errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()):
		res.Code, res.Message = DiagTimeout, "the server did not answer in time (asleep, firewalled or off the network)"
	default:
		res.Code, res.Message = DiagRefused, "the server could not be reached"
	}
	return res
}

// ToHealth converts a probe into the stored health record.
func (r EndpointProbe) ToHealth(at time.Time) store.EndpointHealth {
	h := store.EndpointHealth{State: r.State, Latency: r.Latency, CheckedAt: at, Models: r.Models}
	if r.State == store.EndpointOnline || r.State == store.EndpointSlow {
		if r.Code == DiagNoModels {
			h.DiscoveryError = r.Message
		}
		return h
	}
	h.Error = r.Code + ": " + r.Message
	h.DiscoveryError = r.Message
	h.KeepModels = true
	return h
}
