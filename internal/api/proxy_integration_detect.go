package api

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/proxycoexist"
	"github.com/GLINCKER/levelrail/internal/proxyroutes"
	"github.com/GLINCKER/levelrail/internal/store"
)

// proxyContainerInspector is the Docker surface detection needs beyond
// listing containers.
type proxyContainerInspector interface {
	InspectLocalContainer(ctx context.Context, name string) (docker.LocalContainer, error)
	BridgeGatewayIP(ctx context.Context) (string, error)
}

// maxStaticConfigSize bounds a Traefik static configuration read from disk.
const maxStaticConfigSize = 1 << 20

// detectProxy finds the container publishing host port 80 or 443 and reads
// its configuration through the Docker API only.
func (rt *Router) detectProxy(ctx context.Context) proxyroutes.Detection {
	if rt.proxyIntegration != nil && rt.proxyIntegration.detect != nil {
		return rt.proxyIntegration.detect(ctx)
	}
	if rt.execRuntime == nil {
		return proxyroutes.None()
	}
	runtime, err := rt.execRuntime("")
	if err != nil {
		return proxyroutes.None()
	}
	lister, ok := runtime.(LocalContainerLister)
	inspector, ok2 := runtime.(proxyContainerInspector)
	if !ok || !ok2 {
		return proxyroutes.None()
	}
	all, err := lister.ListLocalContainers(ctx)
	if err != nil {
		d := proxyroutes.None()
		d.Missing = []string{"cannot list Docker containers: " + err.Error()}
		return d
	}
	holder, kind := pickProxyHolder(all)
	if holder == "" {
		return proxyroutes.None()
	}
	lc, err := inspector.InspectLocalContainer(ctx, holder)
	if err != nil {
		d := proxyroutes.None()
		d.Kind, d.Container = kind, holder
		d.Missing = []string{"cannot inspect " + holder + ": " + err.Error()}
		return d
	}
	c := proxyContainerFrom(lc, kind)
	if hostGatewayRequested(lc.ExtraHosts) {
		if ip, err := inspector.BridgeGatewayIP(ctx); err == nil {
			c.HostGatewayIP = ip
		}
	}
	if kind == proxyroutes.KindTraefik {
		c.StaticFile, c.StaticConfig = readStaticConfig(proxyroutes.StaticConfigCandidates(c))
	}
	return proxyroutes.Detect(c)
}

// pickProxyHolder prefers a recognised proxy, Traefik first, among running
// containers publishing host port 80 or 443.
func pickProxyHolder(all []docker.LocalContainer) (string, string) {
	var name, kind string
	for _, c := range all {
		if !c.Running || !publishesWebPort(c) {
			continue
		}
		k, ok := proxycoexist.KindFromImage(c.Image)
		switch {
		case ok && k == proxycoexist.Traefik:
			return c.Name, proxyroutes.KindTraefik
		case ok && kind == "":
			name, kind = c.Name, string(k)
		}
	}
	return name, kind
}

func publishesWebPort(c docker.LocalContainer) bool {
	for _, p := range c.Published {
		if p == 80 || p == 443 {
			return true
		}
	}
	return false
}

func proxyContainerFrom(lc docker.LocalContainer, kind string) proxyroutes.Container {
	c := proxyroutes.Container{
		Name: lc.Name, Image: lc.Image, Kind: kind, Args: lc.Args, ExtraHosts: lc.ExtraHosts,
		NetworkMode: lc.NetworkMode, Gateway: firstGateway(lc),
	}
	for _, m := range lc.Mounts {
		c.Mounts = append(c.Mounts, proxyroutes.Mount{Source: m.Source, Destination: m.Destination})
	}
	for _, m := range lc.Mappings {
		c.Mappings = append(c.Mappings, proxyroutes.PortMap{Private: m.Private, Public: m.Public})
	}
	return c
}

func hostGatewayRequested(extra []string) bool {
	for _, h := range extra {
		if strings.HasSuffix(h, ":host-gateway") || strings.HasSuffix(h, "=host-gateway") {
			return true
		}
	}
	return false
}

// readStaticConfig returns the first candidate that is a regular file.
func readStaticConfig(candidates []string) (string, []byte) {
	for _, p := range candidates {
		fi, err := os.Lstat(p)
		if err != nil || !fi.Mode().IsRegular() {
			continue
		}
		f, err := os.Open(p) //nolint:gosec // a path under the proxy's own mount, read only
		if err != nil {
			continue
		}
		body, err := io.ReadAll(io.LimitReader(f, maxStaticConfigSize))
		_ = f.Close()
		if err == nil {
			return p, body
		}
	}
	return "", nil
}

// proxySteps derives the setup checklist from the current state.
func (rt *Router) proxySteps(ctx context.Context, det proxyroutes.Detection, ingress store.IngressSettings, plan proxyroutes.Plan, domains []proxyDomainResource) []proxyStepResource {
	steps := []proxyStepResource{detectStep(det), tlsStep(ingress), routesStep(plan, domains)}
	steps = append(steps, rt.dnsStep(ctx, domains), verifyStep(domains))
	return steps
}

func detectStep(det proxyroutes.Detection) proxyStepResource {
	s := proxyStepResource{ID: proxyStepDetect}
	switch {
	case det.Kind == proxyroutes.KindNone:
		s.State, s.Detail = proxyStepBlocked, strings.Join(det.Missing, "; ")
	case !det.Complete:
		s.State, s.Detail = proxyStepError, strings.Join(det.Missing, "; ")
	default:
		s.State = proxyStepDone
		s.Detail = fmt.Sprintf("%s in container %s, routes go to %s", det.Kind, det.Container, det.DynamicDir)
	}
	return s
}

func tlsStep(ingress store.IngressSettings) proxyStepResource {
	if ingress.TLSTerminatedUpstream {
		return proxyStepResource{ID: proxyStepTLSUpstream, State: proxyStepDone,
			Detail: fmt.Sprintf("the proxy terminates TLS; links use port %d", ingress.PublicLinkPort())}
	}
	return proxyStepResource{ID: proxyStepTLSUpstream, State: proxyStepTodo,
		Detail: "setup turns on tls_terminated_upstream so this instance stops requesting certificates"}
}

func routesStep(plan proxyroutes.Plan, domains []proxyDomainResource) proxyStepResource {
	s := proxyStepResource{ID: proxyStepRoutes}
	if !plan.Settings.Enabled() {
		s.State, s.Detail = proxyStepTodo, "managed routes are off"
		return s
	}
	if len(domains) == 0 {
		s.State, s.Detail = proxyStepTodo, "no domains are routed yet: add a domain to an app or set the primary domain"
		return s
	}
	pending := 0
	for _, d := range domains {
		switch d.State {
		case proxyroutes.StateError:
			s.State, s.Detail = proxyStepError, d.Domain+": "+d.LastError
			return s
		case proxyroutes.StateMissing, proxyroutes.StateStale:
			pending++
		}
	}
	if pending > 0 {
		s.State, s.Detail = proxyStepTodo, fmt.Sprintf("%d of %d route files are not current yet; apply writes them", pending, len(domains))
		return s
	}
	s.State, s.Detail = proxyStepDone, fmt.Sprintf("%d route files written to %s", len(domains), plan.Settings.DynamicDir)
	return s
}

func (rt *Router) dnsStep(ctx context.Context, domains []proxyDomainResource) proxyStepResource {
	s := proxyStepResource{ID: proxyStepDNS}
	if len(domains) == 0 {
		s.State, s.Detail = proxyStepTodo, "no domains to check"
		return s
	}
	if rt.publicHost == "" {
		s.State, s.Detail = proxyStepBlocked, "this server's public address is unknown (APP_PUBLIC_HOST), so DNS cannot be compared"
		return s
	}
	timeout := defaultProxyVerifyTimeout
	if rt.proxyIntegration != nil && rt.proxyIntegration.verifyTimeout > 0 {
		timeout = rt.proxyIntegration.verifyTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	wrong := make([]bool, len(domains))
	var wg sync.WaitGroup
	for i, d := range domains {
		wg.Add(1)
		go func(i int, domain string) {
			defer wg.Done()
			ips, err := rt.lookupHost(ctx, domain)
			wrong[i] = err != nil || !containsString(ips, rt.publicHost)
		}(i, d.Domain)
	}
	wg.Wait()
	var pending []string
	for i, w := range wrong {
		if w {
			pending = append(pending, domains[i].Domain)
		}
	}
	if len(pending) > 0 {
		s.State, s.Detail = proxyStepTodo, "point an A record at "+rt.publicHost+" for: "+strings.Join(pending, ", ")
		return s
	}
	s.State, s.Detail = proxyStepDone, "every domain resolves to "+rt.publicHost
	return s
}

func verifyStep(domains []proxyDomainResource) proxyStepResource {
	s := proxyStepResource{ID: proxyStepVerify, State: proxyStepTodo, Detail: "not verified yet"}
	checked := 0
	for _, d := range domains {
		if d.CheckedAt == "" {
			continue
		}
		checked++
		if !d.Reachable || d.Certificate == nil || !d.Certificate.Valid {
			reason := d.LastError
			if reason == "" {
				reason = "no trusted certificate was served"
			}
			s.State, s.Detail = proxyStepError, d.Domain+": "+reason
			return s
		}
	}
	if checked > 0 && checked == len(domains) {
		s.State, s.Detail = proxyStepDone, fmt.Sprintf("%d domains answer over HTTPS with a trusted certificate", checked)
	}
	return s
}
