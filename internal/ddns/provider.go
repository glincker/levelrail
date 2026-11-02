package ddns

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/GLINCKER/levelrail/internal/heartbeat"
	"github.com/GLINCKER/levelrail/internal/secretmask"
	"github.com/GLINCKER/levelrail/kit/netguard"
)

// Provider writes the address to one DNS target.
type Provider interface {
	Update(ctx context.Context, ip string) error
}

const ipPlaceholder = "{ip}"

// HTTPProvider calls an operator-supplied update URL with {ip} substituted.
type HTTPProvider struct {
	template string
	client   *http.Client
	accept   func(body string) bool
}

// NewHTTPProvider validates the template and registers it with the log masker. accept may be nil (any 2xx).
func NewHTTPProvider(template string, cfg Config, accept func(string) bool) (*HTTPProvider, error) {
	template = strings.TrimSpace(template)
	if !strings.Contains(template, ipPlaceholder) {
		return nil, errors.New("ddns: update url needs an {ip} placeholder")
	}
	probe := strings.ReplaceAll(template, ipPlaceholder, "203.0.113.1")
	u, err := url.Parse(probe)
	if err != nil || u.Hostname() == "" {
		return nil, errors.New("ddns: update url is not valid")
	}
	if u.Scheme != "https" && (u.Scheme != "http" || !cfg.AllowInsecure) {
		return nil, errors.New("ddns: update url must use https")
	}
	if !cfg.AllowPrivate {
		if err := netguard.ValidateURL(context.Background(), probe); err != nil {
			return nil, errors.New("ddns: update url destination is not allowed")
		}
	}
	secretmask.Add(template)
	for _, vals := range u.Query() {
		for _, v := range vals {
			if v != "" && v != "203.0.113.1" {
				secretmask.Add(v)
			}
		}
	}
	return &HTTPProvider{template: template, client: heartbeat.NewClient(cfg.AllowPrivate, cfg.Timeout), accept: accept}, nil
}

// NewDuckDNS is the generic provider preset for the DuckDNS update endpoint.
func NewDuckDNS(domains, token string, cfg Config) (*HTTPProvider, error) {
	secretmask.Add(token)
	tpl := "https://www.duckdns.org/update?domains=" + escape(domains) + "&token=" + escape(token) + "&ip=" + ipPlaceholder
	return NewHTTPProvider(tpl, cfg, func(body string) bool { return strings.HasPrefix(strings.TrimSpace(body), "OK") })
}

// Update sends the request. Errors never carry the URL.
func (p *HTTPProvider) Update(ctx context.Context, ip string) error {
	target := strings.ReplaceAll(p.template, ipPlaceholder, url.QueryEscape(ip))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return errors.New("update request could not be built")
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return errors.New(secretmask.Mask(heartbeat.Scrub(err).Error()))
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("provider answered status %d", resp.StatusCode)
	}
	if p.accept != nil && !p.accept(string(body)) {
		return errors.New("provider rejected the update")
	}
	return nil
}
