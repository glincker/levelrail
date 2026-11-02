package exposure

import (
	"context"
	"fmt"
	"net/netip"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// DriftKind names one way declared exposure and the host disagree.
type DriftKind string

// Drift kinds.
const (
	DriftUnmanagedPublicPort   DriftKind = "unmanaged_public_port"
	DriftRestrictionMissing    DriftKind = "restriction_missing"
	DriftRestrictionUndeclared DriftKind = "restriction_undeclared"
	DriftListenerUndeclared    DriftKind = "listener_undeclared"
)

// DriftFinding is one disagreement between declared and actual exposure.
type DriftFinding struct {
	Kind     DriftKind `json:"kind"`
	Severity Severity  `json:"severity"`
	Port     int       `json:"port"`
	Protocol string    `json:"protocol"`
	Subject  string    `json:"subject,omitempty"`
	Message  string    `json:"message"`
	Fix      string    `json:"fix,omitempty"`
	// Rules are the DOCKER-USER rules involved, verbatim.
	Rules []string `json:"rules,omitempty"`
}

// Listener is one listening socket as `ss` reports it.
type Listener struct {
	Protocol string
	Address  string
	Port     int
	// Process is the owning process name from ss -p, empty when ss could not say.
	Process string
	PID     int
}

// DriftInput is everything a drift check compares. It is plain data so the
// check never touches the host.
type DriftInput struct {
	Findings []Finding
	Declared []Restriction
	Chain    Chain
	// Listeners is nil when sockets could not be read, which skips that check.
	Listeners []Listener
	// Known lists ports expected to listen publicly (control plane, ingress, SSH).
	Known  []int
	Prefix string
}

// CheckDrift compares declared exposure with the host. It only reads.
func CheckDrift(in DriftInput) []DriftFinding {
	var out []DriftFinding
	published := map[string]bool{}
	for _, f := range in.Findings {
		published[restrictionID(f.Protocol, f.HostPort)] = true
		if f.Owner.Kind == OwnerUnmanaged && f.Class == ClassExposed && f.ImageKind != KindWeb {
			out = append(out, DriftFinding{
				Kind: DriftUnmanagedPublicPort, Severity: f.Severity, Port: f.HostPort, Protocol: f.Protocol, Subject: f.Container,
				Message: fmt.Sprintf("container %s publishes %d/%s to the internet and nothing in the platform declares it", f.Container, f.HostPort, f.Protocol),
				Fix:     "restrict it with a firewall restriction, or stop publishing the port",
			})
		}
	}
	if in.Chain.Readable {
		out = append(out, chainDrift(in)...)
	}
	for _, l := range in.Listeners {
		if publicAddr(l.Address) && !published[restrictionID(l.Protocol, l.Port)] && !slices.Contains(in.Known, l.Port) {
			out = append(out, DriftFinding{
				Kind: DriftListenerUndeclared, Severity: SeverityMedium, Port: l.Port, Protocol: l.Protocol, Subject: l.Address,
				Message: fmt.Sprintf("%s listens on %s:%d/%s and no container or platform service declares it", listenerOwner(l), l.Address, l.Port, l.Protocol),
				Fix:     listenerFix(l),
			})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if r := severityRank(out[i].Severity) - severityRank(out[j].Severity); r != 0 {
			return r > 0
		}
		if out[i].Port != out[j].Port {
			return out[i].Port < out[j].Port
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

func restrictionID(proto string, port int) string { return proto + "/" + strconv.Itoa(port) }

// chainDrift compares declared restrictions with the rules actually present.
func chainDrift(in DriftInput) []DriftFinding {
	var out []DriftFinding
	tagBase := in.Prefix + tagInfix
	declared := map[string]bool{}
	for _, d := range in.Declared {
		proto := d.Protocol
		if proto == "" {
			proto = "tcp"
		}
		id := restrictionID(proto, d.Port)
		declared[id] = true
		var have []ChainRule
		for _, r := range in.Chain.Rules {
			if r.Comment == tagBase+id {
				have = append(have, r)
			}
		}
		if !slices.ContainsFunc(have, ChainRule.terminal) {
			out = append(out, DriftFinding{
				Kind: DriftRestrictionMissing, Severity: SeverityHigh, Port: d.Port, Protocol: proto,
				Message: fmt.Sprintf("a restriction on %s is recorded but its DOCKER-USER rules are not on this host, so the port is open to everyone", id),
				Fix:     fmt.Sprintf("levelrail-cli firewall restrict --port %d --protocol %s --allow <sources> --apply", d.Port, proto),
			})
		}
	}
	seen := map[string]bool{}
	for _, r := range in.Chain.Rules {
		if r.Complex && r.Comment == "" {
			continue
		}
		if id, ok := strings.CutPrefix(r.Comment, tagBase); ok {
			if !declared[id] && !seen[id] {
				seen[id] = true
				proto, p, _ := strings.Cut(id, "/")
				port, _ := strconv.Atoi(p)
				out = append(out, DriftFinding{
					Kind: DriftRestrictionUndeclared, Severity: SeverityLow, Port: port, Protocol: proto, Subject: r.Comment,
					Message: fmt.Sprintf("tagged DOCKER-USER rules for %s exist but no restriction is recorded for it", id),
					Fix:     fmt.Sprintf("levelrail-cli firewall unrestrict --port %d --protocol %s, or restrict it again to record it", port, proto),
					Rules:   []string{r.Raw},
				})
			}
			continue
		}
		port := r.OrigDPort
		if port == 0 {
			port = r.DPort
		}
		if port == 0 || (!r.terminal() && !r.passes()) {
			continue
		}
		proto := r.Proto
		if proto == "" {
			proto = "tcp"
		}
		id := "hand/" + restrictionID(proto, port) + "/" + r.Target + "/" + r.Source
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, DriftFinding{
			Kind: DriftRestrictionUndeclared, Severity: SeverityInfo, Port: port, Protocol: proto, Subject: "untagged",
			Message: fmt.Sprintf("a hand-made DOCKER-USER rule %s on %s is not managed by the platform", r.Target, restrictionID(proto, port)),
			Fix:     "adopt it by creating the same restriction here, or leave it alone",
			Rules:   []string{r.Raw},
		})
	}
	return out
}

// publicAddr reports whether a listening address accepts outside connections.
func publicAddr(a string) bool {
	a = strings.TrimSpace(a)
	if a == "*" || a == "" {
		return true
	}
	if i := strings.IndexByte(a, '%'); i >= 0 {
		a = a[:i]
	}
	ip, err := netip.ParseAddr(strings.Trim(a, "[]"))
	if err != nil {
		return false
	}
	return !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsLinkLocalUnicast()
}

// listenerOwner names the owning process, or "something" when ss did not say.
func listenerOwner(l Listener) string {
	switch {
	case l.Process != "" && l.PID > 0:
		return fmt.Sprintf("process %s (pid %d)", l.Process, l.PID)
	case l.Process != "":
		return "process " + l.Process
	}
	return "something"
}

func listenerFix(l Listener) string {
	if l.Process != "" {
		return fmt.Sprintf("stop %s or bind it to a private address", l.Process)
	}
	return "find the process with ss -ltnup (as root), then stop it or bind it to a private address"
}

// parseSSProcess reads the first process entry (name and pid) of an ss -p line.
func parseSSProcess(fields []string) (string, int) {
	for _, f := range fields {
		rest, ok := strings.CutPrefix(f, "users:((\"")
		if !ok {
			continue
		}
		name, tail, ok := strings.Cut(rest, "\"")
		if !ok {
			return "", 0
		}
		pid := 0
		if _, after, ok := strings.Cut(tail, "pid="); ok {
			digits, _, _ := strings.Cut(after, ",")
			digits = strings.TrimRight(digits, ")")
			pid, _ = strconv.Atoi(digits)
		}
		return name, pid
	}
	return "", 0
}

// ParseListeners parses `ss -H -ltnup` output, with or without the process column. Lines it cannot read are skipped.
func ParseListeners(out string) []Listener {
	var ls []Listener
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 5 || (f[0] != "tcp" && f[0] != "udp") {
			continue
		}
		i := strings.LastIndexByte(f[4], ':')
		if i < 0 {
			continue
		}
		port, err := strconv.Atoi(f[4][i+1:])
		if err != nil {
			continue
		}
		proc, pid := parseSSProcess(f[5:])
		ls = append(ls, Listener{Protocol: f[0], Address: f[4][:i], Port: port, Process: proc, PID: pid})
	}
	return ls
}

// ReadListeners lists listening sockets, or nil when they cannot be read.
func (m *Manager) ReadListeners(ctx context.Context) []Listener {
	if m.goos != "linux" {
		return nil
	}
	if _, err := m.lookPath("ss"); err != nil {
		return nil
	}
	out, err := m.run(ctx, "ss", "-H", "-ltnup")
	if err != nil {
		return nil
	}
	return ParseListeners(string(out))
}

// Protected returns the ports restrictions may never touch.
func (m *Manager) Protected() []int { return slices.Clone(m.protected) }
