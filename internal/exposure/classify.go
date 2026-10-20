package exposure

import (
	"fmt"
	"net"
	"net/netip"
	"sort"
	"strings"
)

var cgnat = netip.MustParsePrefix("100.64.0.0/10")

type bindKind int

const (
	bindLoopback bindKind = iota
	bindPrivate
	bindPublic
)

// bindKindOf classifies one host bind address. Empty, 0.0.0.0 and :: bind
// every interface and count as public.
func bindKindOf(host string) bindKind {
	host = strings.TrimSpace(host)
	if host == "" {
		return bindPublic
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return bindPublic
	}
	ip = ip.Unmap()
	switch {
	case ip.IsUnspecified():
		return bindPublic
	case ip.IsLoopback():
		return bindLoopback
	case ip.IsPrivate(), ip.IsLinkLocalUnicast(), cgnat.Contains(ip):
		return bindPrivate
	}
	return bindPublic
}

// Verdict is the reachability decision for one published port.
type Verdict struct {
	Class   Class
	Allowed []string
	Rules   []string
	Managed bool
	Reason  string
}

// Classify decides how reachable a binding is given the DOCKER-USER chain.
// It never reports restricted unless a simple matching DROP or REJECT exists.
func Classify(binds []string, hostPort, containerPort int, proto string, chain Chain, prefix string) Verdict {
	worst := bindLoopback
	for _, b := range binds {
		if k := bindKindOf(b); k > worst {
			worst = k
		}
	}
	switch worst {
	case bindLoopback:
		return Verdict{Class: ClassLoopback}
	case bindPrivate:
		return Verdict{Class: ClassPrivate}
	}
	if !chain.Readable {
		return Verdict{Class: ClassUnknown, Reason: chain.Reason}
	}
	var matched []ChainRule
	for _, r := range chain.Rules {
		if r.applies(hostPort, containerPort, proto) {
			matched = append(matched, r)
		}
	}
	v := Verdict{Class: ClassExposed}
	for _, r := range matched {
		v.Rules = append(v.Rules, r.Raw)
	}
	var drop *ChainRule
	for i := range matched {
		r := matched[i]
		if r.Complex {
			v.Class = ClassUnknown
			v.Reason = "a rule mentions this port but uses matches this audit cannot interpret"
			return v
		}
		if r.terminal() && drop == nil {
			drop = &matched[i]
		}
	}
	if drop == nil {
		return v
	}
	for _, r := range matched {
		if r.passes() && r.Source != "" && !r.SourceNegated {
			v.Allowed = append(v.Allowed, r.Source)
		}
		if strings.HasPrefix(r.Comment, prefix) {
			v.Managed = true
		}
	}
	if drop.SourceNegated && drop.Source != "" {
		v.Allowed = append(v.Allowed, drop.Source)
	}
	if drop.Source != "" && !drop.SourceNegated {
		v.Class = ClassExposed
		v.Reason = "the matching rule only drops one source, other sources still reach the port"
		return v
	}
	sort.Strings(v.Allowed)
	v.Class = ClassRestricted
	return v
}

var imageKinds = map[string]ImageKind{
	"postgres": KindDatastore, "postgresql": KindDatastore, "pgvector": KindDatastore, "mysql": KindDatastore,
	"mariadb": KindDatastore, "mongo": KindDatastore, "mongodb": KindDatastore, "redis": KindDatastore,
	"valkey": KindDatastore, "memcached": KindDatastore, "clickhouse": KindDatastore, "cassandra": KindDatastore,
	"couchdb": KindDatastore, "influxdb": KindDatastore, "rabbitmq": KindDatastore, "etcd": KindDatastore,
	"minio": KindDatastore, "nats": KindDatastore, "kafka": KindDatastore, "zookeeper": KindDatastore,
	"typesense": KindSearch, "elasticsearch": KindSearch, "opensearch": KindSearch, "meilisearch": KindSearch,
	"solr": KindSearch, "qdrant": KindSearch, "weaviate": KindSearch, "milvus": KindSearch,
	"portainer": KindAdmin, "adminer": KindAdmin, "pgadmin4": KindAdmin, "phpmyadmin": KindAdmin,
	"redis-commander": KindAdmin, "mongo-express": KindAdmin, "grafana": KindAdmin, "prometheus": KindAdmin,
	"nginx": KindWeb, "caddy": KindWeb, "traefik": KindWeb, "httpd": KindWeb, "apache": KindWeb,
}

var portKinds = map[int]ImageKind{
	5432: KindDatastore, 3306: KindDatastore, 6379: KindDatastore, 27017: KindDatastore, 11211: KindDatastore,
	9000: KindOther, 8123: KindDatastore, 9042: KindDatastore, 5984: KindDatastore, 8086: KindDatastore,
	5672: KindDatastore, 2379: KindDatastore, 9200: KindSearch, 8108: KindSearch, 7700: KindSearch,
	6333: KindSearch, 2375: KindDockerAPI, 2376: KindDockerAPI,
}

// ImageKindOf guesses what a container runs from its image repository name,
// falling back to the container port.
func ImageKindOf(image string, containerPort int) ImageKind {
	repo := image
	if i := strings.IndexAny(repo, "@"); i >= 0 {
		repo = repo[:i]
	}
	if i := strings.LastIndex(repo, "/"); i >= 0 {
		repo = repo[i+1:]
	}
	if i := strings.Index(repo, ":"); i >= 0 {
		repo = repo[:i]
	}
	if k, ok := imageKinds[strings.ToLower(repo)]; ok {
		return k
	}
	if k, ok := portKinds[containerPort]; ok {
		return k
	}
	return KindOther
}

func sensitive(k ImageKind) bool {
	return k == KindDatastore || k == KindSearch || k == KindAdmin || k == KindDockerAPI
}

// SeverityFor ranks a finding by what is exposed, not just that it is.
func SeverityFor(class Class, kind ImageKind, owner Owner) Severity {
	switch class {
	case ClassExposed:
		switch {
		case sensitive(kind) && owner.Intentional:
			return SeverityMedium
		case sensitive(kind):
			return SeverityHigh
		case kind == KindWeb && owner.Kind != OwnerUnmanaged:
			return SeverityInfo
		case kind == KindWeb || owner.Kind == OwnerApp:
			return SeverityLow
		}
		return SeverityMedium
	case ClassUnknown:
		if sensitive(kind) {
			return SeverityMedium
		}
		return SeverityLow
	}
	return SeverityInfo
}

var kindNoun = map[ImageKind]string{
	KindDatastore: "a database or data store", KindSearch: "a search engine", KindAdmin: "an admin console",
	KindDockerAPI: "the Docker API", KindWeb: "a web server", KindOther: "a service",
}

var kindRisk = map[ImageKind]string{
	KindDatastore: "Anyone who can reach it can try to read, change or delete your data.",
	KindSearch:    "Anyone who can reach it can read or wipe indexed data, and many search engines ship without authentication.",
	KindAdmin:     "Admin consoles hand control of your systems to whoever logs in, and they are scanned for constantly.",
	KindDockerAPI: "Unauthenticated access to the Docker API is full control of this server.",
	KindWeb:       "Web ports are normally meant to be public, but should sit behind the ingress so you get TLS and logs.",
	KindOther:     "Check that this service is meant to be public and has its own authentication.",
}

// Explain returns the plain-language explanation and the suggested action.
func Explain(f Finding, outsideLabel string) (explanation, recommendation string) {
	who := "container " + f.Container
	switch f.Owner.Kind {
	case OwnerDatabase:
		who = "your managed database " + f.Owner.Name
	case OwnerApp:
		who = "your app " + f.Owner.Name
	}
	port := fmt.Sprintf("%d/%s", f.HostPort, f.Protocol)
	switch f.Class {
	case ClassLoopback:
		return fmt.Sprintf("Port %s of %s only listens on this server itself, so nothing outside can connect.", port, who), ""
	case ClassPrivate:
		return fmt.Sprintf("Port %s of %s listens on a private network address only, not on the public internet.", port, who), ""
	case ClassRestricted:
		src := "an allow-list"
		if len(f.AllowedSources) > 0 {
			src = strings.Join(f.AllowedSources, ", ")
		}
		return fmt.Sprintf("Port %s of %s is limited by a firewall rule in the DOCKER-USER chain to: %s.", port, who, src), ""
	case ClassUnknown:
		return fmt.Sprintf("Port %s of %s (%s) is published on every network interface, but this audit could not tell whether a firewall rule limits it. %s",
				port, who, kindNoun[f.ImageKind], kindRisk[f.ImageKind]),
			"Read the host firewall with root rights, or add an allow-list for this port."
	}
	e := fmt.Sprintf("Port %s of %s (%s) is published on %s and no DOCKER-USER rule restricts it. Docker bypasses ufw for published ports, so a host firewall that looks active does not cover this. %s",
		port, who, kindNoun[f.ImageKind], strings.Join(f.Binds, ", "), kindRisk[f.ImageKind])
	if outsideLabel != "" {
		e += " " + outsideLabel
	}
	if f.Owner.Kind == OwnerDatabase && !f.Owner.Intentional {
		return e, "Turn off public access for this database, or allow only your other servers."
	}
	if f.ImageKind == KindWeb && f.Owner.Kind != OwnerUnmanaged {
		return e, "No action needed if this is intended. Otherwise route it through the ingress."
	}
	return e, "Allow only the servers that need this port."
}

// BindsOf collects the distinct bind addresses, wildcard spelled explicitly.
func BindsOf(ports []PortBinding) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range ports {
		h := p.HostIP
		if h == "" {
			h = "0.0.0.0"
		}
		if !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	sort.Strings(out)
	return out
}

// Audit turns a container list into findings, one per published (port,
// protocol, container) and sorted worst first.
func Audit(containers []Container, chain Chain, prefix string) []Finding {
	type key struct {
		id    string
		port  int
		proto string
	}
	group := map[key][]PortBinding{}
	byID := map[string]Container{}
	var order []key
	for _, c := range containers {
		if !c.Running {
			continue
		}
		byID[c.ID] = c
		for _, p := range c.Ports {
			if p.HostPort == 0 {
				continue
			}
			proto := p.Protocol
			if proto == "" {
				proto = "tcp"
			}
			k := key{c.ID, p.HostPort, proto}
			if _, ok := group[k]; !ok {
				order = append(order, k)
			}
			group[k] = append(group[k], p)
		}
	}
	out := make([]Finding, 0, len(order))
	for _, k := range order {
		c := byID[k.id]
		ports := group[k]
		binds := BindsOf(ports)
		v := Classify(binds, k.port, ports[0].ContainerPort, k.proto, chain, prefix)
		kind := ImageKindOf(c.Image, ports[0].ContainerPort)
		f := Finding{
			Container: c.Name, Image: c.Image, Owner: c.Owner, ImageKind: kind,
			Protocol: k.proto, HostPort: k.port, ContainerPort: ports[0].ContainerPort, Binds: binds,
			Class: v.Class, AllowedSources: v.Allowed, Rules: v.Rules, Managed: v.Managed,
			Severity: SeverityFor(v.Class, kind, c.Owner),
		}
		f.Explanation, f.Recommendation = Explain(f, "")
		out = append(out, f)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if r := severityRank(out[i].Severity) - severityRank(out[j].Severity); r != 0 {
			return r > 0
		}
		return out[i].HostPort < out[j].HostPort
	})
	return out
}

func severityRank(s Severity) int {
	switch s {
	case SeverityHigh:
		return 3
	case SeverityMedium:
		return 2
	case SeverityLow:
		return 1
	}
	return 0
}

// ValidCIDR reports whether s parses as an IP or CIDR.
func ValidCIDR(s string) bool {
	if _, _, err := net.ParseCIDR(s); err == nil {
		return true
	}
	return net.ParseIP(s) != nil
}
