package proxyroutes

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// KindHAProxy is the proxy kind of an HAProxy container.
const KindHAProxy = "haproxy"

// Exec runs a command inside a proxy container through the Docker Engine
// API and returns its standard output. A non-zero exit is an *ExecError.
type Exec interface {
	Exec(ctx context.Context, container string, cmd []string) (string, error)
}

// ExecError is a command that ran and exited non-zero; Output is what it
// printed, verbatim.
type ExecError struct {
	Output string
	Err    error
}

func (e *ExecError) Error() string {
	if e.Output != "" {
		return e.Output
	}
	return e.Err.Error()
}

func (e *ExecError) Unwrap() error { return e.Err }

// Provider is one kind of external proxy: how to read it, what route file to
// write for a domain, and how to make the proxy load the files.
type Provider interface {
	Kind() string
	// Mode is the saved integration mode that turns the provider on, or ""
	// for a provider that only generates a snippet to apply by hand.
	Mode() string
	// Ext is the extension of the files the provider writes.
	Ext() string
	Detect(c Container) Detection
	Render(ns Namespace, r Route) ([]byte, error)
	// Validate asks the proxy to check the files on disk before it loads them.
	Validate(ctx context.Context, ex Exec, container string) error
	// Reload makes the proxy load files that passed Validate.
	Reload(ctx context.Context, ex Exec, container string) error
}

// Managed reports whether p writes files into the proxy's directory.
func Managed(p Provider) bool { return p.Mode() != "" }

// Providers lists every provider in the order the picker shows them.
func Providers() []Provider {
	return []Provider{traefikProvider{}, nginxProvider{}, snippetProvider{kind: KindCaddy}, snippetProvider{kind: KindHAProxy}}
}

// ProviderFor returns the provider for a proxy kind.
func ProviderFor(kind string) (Provider, bool) {
	for _, p := range Providers() {
		if p.Kind() == kind {
			return p, true
		}
	}
	return nil, false
}

// ProviderForMode returns the managed provider a saved mode selects.
func ProviderForMode(mode string) (Provider, bool) {
	for _, p := range Providers() {
		if p.Mode() != "" && p.Mode() == mode {
			return p, true
		}
	}
	return nil, false
}

type traefikProvider struct{}

func (traefikProvider) Kind() string                                 { return KindTraefik }
func (traefikProvider) Mode() string                                 { return store.ProxyIntegrationTraefikFile }
func (traefikProvider) Ext() string                                  { return DefaultFileExt }
func (traefikProvider) Detect(c Container) Detection                 { return detectTraefik(c) }
func (traefikProvider) Render(ns Namespace, r Route) ([]byte, error) { return ns.Render(r) }
func (traefikProvider) Validate(context.Context, Exec, string) error { return nil }
func (traefikProvider) Reload(context.Context, Exec, string) error   { return nil }

// snippetProvider renders configuration for a proxy this instance does not
// manage: shown to the operator, never written.
type snippetProvider struct{ kind string }

func (p snippetProvider) Kind() string { return p.kind }
func (snippetProvider) Mode() string   { return "" }
func (snippetProvider) Ext() string    { return ".conf" }

func (p snippetProvider) Detect(c Container) Detection {
	d := Detection{Kind: c.Kind, Container: c.Name, Image: c.Image, HostNetwork: c.NetworkMode == "host"}
	for _, m := range c.Mappings {
		d.PublishedPorts = append(d.PublishedPorts, m.Public)
	}
	d.PublishedPorts = uniqueInts(d.PublishedPorts)
	d.detectUpstream(c)
	d.Missing = append(d.Missing, "managed routes are not available for "+c.Kind+": apply the generated snippet by hand")
	d.finish()
	return d
}

func (p snippetProvider) Render(ns Namespace, r Route) ([]byte, error) {
	d, err := ValidateDomain(r.Domain)
	if err != nil {
		return nil, err
	}
	if err := ValidateUpstream(r.Upstream); err != nil {
		return nil, err
	}
	var b strings.Builder
	switch p.kind {
	case KindCaddy:
		fmt.Fprintf(&b, "%s {\n\treverse_proxy %s\n}\n", d, r.Upstream)
	case KindHAProxy:
		obj := strings.ReplaceAll(string(ns)+"_"+d, ".", "_")
		fmt.Fprintf(&b, "# in your https frontend:\n\tacl %s_host hdr(host) -i %s\n\tuse_backend %s if %s_host\n\n", obj, d, obj, obj)
		fmt.Fprintf(&b, "backend %s\n\toption forwardfor\n\thttp-request set-header X-Forwarded-Proto https\n\thttp-request set-header X-Forwarded-Host %%[req.hdr(host)]\n\tserver upstream %s\n", obj, r.Upstream)
	default:
		return nil, fmt.Errorf("no snippet for %q", p.kind)
	}
	return []byte(b.String()), nil
}

var errManual = errors.New("this proxy is configured by hand, nothing to validate")

func (snippetProvider) Validate(context.Context, Exec, string) error { return errManual }
func (snippetProvider) Reload(context.Context, Exec, string) error   { return errManual }
