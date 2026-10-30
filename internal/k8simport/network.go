package k8simport

import (
	"encoding/base64"
	"fmt"
	"sort"
	"strings"
)

func decodeBase64(s string) (string, error) {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func matches(selector, labels map[string]string) bool {
	if len(selector) == 0 {
		return false
	}
	for k, v := range selector {
		if labels[k] != v {
			return false
		}
	}
	return true
}

func (im *importer) selected(ns string, selector map[string]string) []*app {
	var out []*app
	for _, a := range im.apps {
		if a.ns == ns && a.kind != "Job" && a.kind != "CronJob" && matches(selector, a.podLabels) {
			out = append(out, a)
		}
	}
	return out
}

func (a *app) resolveTarget(t intOrStr, fallback int) int {
	if t.Int > 0 {
		return t.Int
	}
	if t.Str != "" {
		for _, p := range a.containerPorts {
			if p.Name == t.Str {
				return p.ContainerPort
			}
		}
		return 0
	}
	return fallback
}

func httpish(p servicePort) bool {
	return p.Port == 80 || p.Port == 443 || p.Port == 8080 || strings.HasPrefix(p.Name, "http") || p.Name == "web"
}

func (im *importer) service(o object) {
	var ss serviceSpec
	if err := o.Spec.Decode(&ss); err != nil {
		im.res.add(SeverityChangesBehavior, o.Kind, o.Metadata.Name, "spec", "spec does not decode: "+trimDecodeErr(err))
		return
	}
	if ss.Type == "ExternalName" {
		im.res.add(SeverityChangesBehavior, o.Kind, o.Metadata.Name, "spec.type", "ExternalName services are not supported, point the env var at the external host")
		return
	}
	targets := im.selected(o.ns(), ss.Selector)
	if len(targets) == 0 {
		im.res.add(SeverityChangesBehavior, o.Kind, o.Metadata.Name, "spec.selector", "selector matches no workload in the input")
		return
	}
	exposed := ss.Type == "NodePort" || ss.Type == "LoadBalancer"
	for _, a := range targets {
		if a.name != im.appName(o.ns(), o.Metadata.Name) {
			im.res.add(SeverityChangesBehavior, o.Kind, o.Metadata.Name, "metadata.name", "reach this workload by its app name "+a.name+", not the service name")
		}
		if !a.portFromService {
			if tp := a.routedTarget(ss); tp > 0 {
				a.port, a.portFromService = tp, true
			}
		}
		for _, p := range ss.Ports {
			tp := a.resolveTarget(p.TargetPort, p.Port)
			udp := strings.EqualFold(p.Protocol, "UDP")
			switch {
			case tp == 0:
			case exposed && (udp || tp != a.port):
				host := p.NodePort
				if host == 0 {
					host = p.Port
				}
				proto := "tcp"
				if udp {
					proto = "udp"
				}
				a.published = append(a.published, pubPort{container: tp, host: host, protocol: proto})
				im.res.add(SeverityChangesBehavior, o.Kind, o.Metadata.Name, "spec.ports."+p.Name, fmt.Sprintf("published on host port %d: it binds the platform default address, set bind_address to expose it publicly", host))
			case udp:
				im.res.add(SeverityChangesBehavior, o.Kind, o.Metadata.Name, "spec.ports."+p.Name, "UDP port of a ClusterIP service is not reachable between apps, use NodePort to publish it")
			case !exposed && tp != a.port:
				im.res.add(SeverityIgnoredSafe, o.Kind, o.Metadata.Name, "spec.ports."+p.Name, fmt.Sprintf("extra internal port %d is not routed", tp))
			}
		}
	}
}

// routedTarget picks the service port the platform route should reach.
func (a *app) routedTarget(ss serviceSpec) int {
	first := 0
	for _, p := range ss.Ports {
		if strings.EqualFold(p.Protocol, "UDP") {
			continue
		}
		tp := a.resolveTarget(p.TargetPort, p.Port)
		if tp == 0 {
			continue
		}
		if httpish(p) {
			return tp
		}
		if first == 0 {
			first = tp
		}
	}
	return first
}

func (im *importer) ingress(o object) {
	var is ingressSpec
	if err := o.Spec.Decode(&is); err != nil {
		im.res.add(SeverityChangesBehavior, o.Kind, o.Metadata.Name, "spec", "spec does not decode: "+trimDecodeErr(err))
		return
	}
	if len(is.TLS.Content) > 0 {
		im.res.add(SeverityIgnoredSafe, o.Kind, o.Metadata.Name, "spec.tls", "certificates are issued automatically for domains")
	}
	for k := range o.Metadata.Annotations {
		sev := SeverityIgnoredSafe
		for _, w := range []string{"rewrite", "auth", "whitelist", "allowlist", "snippet", "proxy-body", "redirect", "cors"} {
			if strings.Contains(k, w) {
				sev = SeverityChangesBehavior
			}
		}
		im.res.add(sev, o.Kind, o.Metadata.Name, "metadata.annotations."+k, "controller annotation has no equivalent")
	}
	owners := map[string]string{}
	for _, r := range is.Rules {
		if r.Host == "" || strings.Contains(r.Host, "*") {
			im.res.add(SeverityChangesBehavior, o.Kind, o.Metadata.Name, "spec.rules.host", "rules without an exact host are not imported")
			continue
		}
		for _, p := range r.HTTP.Paths {
			svc, ok := im.services[o.ns()+"/"+p.Backend.Service.Name]
			if !ok {
				im.res.add(SeverityChangesBehavior, o.Kind, o.Metadata.Name, "spec.rules.http.paths", "backend service "+p.Backend.Service.Name+" is not in the input")
				continue
			}
			var ss serviceSpec
			_ = svc.Spec.Decode(&ss)
			if p.Path != "" && p.Path != "/" {
				im.res.add(SeverityChangesBehavior, o.Kind, o.Metadata.Name, "spec.rules.http.paths.path", fmt.Sprintf("path %s on %s: domains route by host only, the whole host goes to the app", p.Path, r.Host))
			}
			for _, a := range im.selected(o.ns(), ss.Selector) {
				if prev, dup := owners[r.Host]; dup && prev != a.name {
					im.res.add(SeverityChangesBehavior, o.Kind, o.Metadata.Name, "spec.rules.host", fmt.Sprintf("host %s routes to both %s and %s by path; only %s gets the domain", r.Host, prev, a.name, prev))
					continue
				}
				if _, dup := owners[r.Host]; !dup {
					owners[r.Host] = a.name
					a.domains = append(a.domains, r.Host)
				}
			}
		}
	}
}

// finish reports unused inputs and renders the documents.
func (im *importer) finish() {
	for _, k := range sortedMap(im.configs) {
		if !im.usedCM[k] {
			im.res.add(SeverityIgnoredSafe, "ConfigMap", k, "", "not referenced by an imported workload, keys: "+strings.Join(sortedKeys(im.configs[k]), ","))
		}
	}
	for _, k := range sortedMap(im.secrets) {
		if !im.usedSec[k] && im.secretTyp[k] == "" || !im.usedSec[k] && im.secretTyp[k] == "Opaque" {
			im.res.add(SeverityIgnoredSafe, "Secret", k, "", "not referenced by an imported workload, not imported, key names: "+strings.Join(sortedKeys(im.secrets[k]), ","))
		}
	}
	keys := make([]string, 0, len(im.pvcs))
	for k := range im.pvcs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		im.res.add(SeverityIgnoredSafe, "PersistentVolumeClaim", im.pvcs[k].Metadata.Name, "spec", "storage size and class are not enforced by named volumes")
		if !im.usedPVC[k] {
			im.res.add(SeverityIgnoredSafe, "PersistentVolumeClaim", im.pvcs[k].Metadata.Name, "", "not mounted by an imported workload")
		}
	}
	for _, a := range im.apps {
		for _, pp := range a.probePorts {
			if tp := a.resolveTarget(pp, 0); tp > 0 && a.port > 0 && tp != a.port {
				im.res.add(SeverityChangesBehavior, a.kind, a.srcName, "probe.port", "probes check the routed port here, not port "+fmt.Sprint(tp))
			}
		}
	}
	im.render()
	im.res.sortFindings()
}

func sortedMap[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
