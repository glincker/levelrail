package exposure

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os/exec"
	"runtime"
	"slices"
	"strconv"
	"strings"
)

const (
	dockerUserChain = "DOCKER-USER"
	tagInfix        = "exposure:"
)

// ErrLockout is wrapped when a restriction would cut off a port the
// platform or the operator's SSH session needs.
var ErrLockout = errors.New("exposure: restriction would lock out a required port")

// Runner executes a command, abstracted so tests never touch a real firewall.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

func realRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput() //nolint:gosec // name is the fixed literal iptables; args are validated ports, CIDRs and a tag
}

// Restriction is a desired allow-list for one published port.
type Restriction struct {
	Port     int      `json:"port"`
	Protocol string   `json:"protocol"`
	Allow    []string `json:"allow"`
}

// Plan is the exact rule set a restriction produces.
type Plan struct {
	Commands []string `json:"commands"`
	Drops    string   `json:"drops"`
	Tag      string   `json:"tag"`
	// Persistence says plainly how long the rules last.
	Persistence string `json:"persistence"`
}

// Manager reads and edits the DOCKER-USER chain, touching only rules that
// carry its own comment tag.
type Manager struct {
	prefix    string
	protected []int
	run       Runner
	lookPath  func(string) (string, error)
	goos      string
}

// NewManager builds a Manager over the real iptables binary. protected lists
// ports that must never be restricted (control plane, ingress, SSH).
func NewManager(prefix string, protected []int) *Manager {
	return &Manager{prefix: prefix, protected: protected, run: realRunner, lookPath: exec.LookPath, goos: runtime.GOOS}
}

// NewFakeManager builds a Manager over fake seams for tests.
func NewFakeManager(prefix string, protected []int, run Runner, lookPath func(string) (string, error), goos string) *Manager {
	return &Manager{prefix: prefix, protected: protected, run: run, lookPath: lookPath, goos: goos}
}

// Prefix returns the rule comment prefix this manager owns.
func (m *Manager) Prefix() string { return m.prefix }

func (m *Manager) tag(proto string, port int) string {
	return m.prefix + tagInfix + proto + "/" + strconv.Itoa(port)
}

// ReadChain reads DOCKER-USER, reporting why it could not when it cannot.
func (m *Manager) ReadChain(ctx context.Context) Chain {
	if m.goos != "linux" {
		return Chain{Reason: "this host is not Linux, so the DOCKER-USER chain is not visible from here"}
	}
	if _, err := m.lookPath("iptables"); err != nil {
		return Chain{Reason: "iptables is not installed here (Docker may be using its nftables mode)"}
	}
	out, err := m.run(ctx, "iptables", "-w", "-S", dockerUserChain)
	if err != nil {
		return Chain{Reason: "could not read the DOCKER-USER chain: " + oneLine(out, err)}
	}
	return ParseChain(string(out))
}

func oneLine(out []byte, err error) string {
	s := strings.TrimSpace(string(out))
	if s == "" {
		s = err.Error()
	}
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}

// Normalize validates a restriction and returns it in canonical form.
func (m *Manager) Normalize(r Restriction) (Restriction, error) {
	if r.Port < 1 || r.Port > 65535 {
		return r, errors.New("port must be between 1 and 65535")
	}
	if r.Protocol == "" {
		r.Protocol = "tcp"
	}
	if r.Protocol != "tcp" && r.Protocol != "udp" {
		return r, errors.New("protocol must be tcp or udp")
	}
	if slices.Contains(m.protected, r.Port) {
		return r, fmt.Errorf("%w: port %d is used by the control plane, its agents, ingress or SSH", ErrLockout, r.Port)
	}
	if len(r.Allow) == 0 {
		return r, errors.New("allow at least one source, an empty allow-list would block everyone")
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(r.Allow))
	for _, a := range r.Allow {
		c, err := canonicalSource(a)
		if err != nil {
			return r, err
		}
		if c == "0.0.0.0/0" {
			return r, errors.New("0.0.0.0/0 allows everyone, which is not a restriction")
		}
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	slices.Sort(out)
	r.Allow = out
	return r, nil
}

func canonicalSource(s string) (string, error) {
	s = strings.TrimSpace(s)
	if p, err := netip.ParsePrefix(s); err == nil {
		if p.Addr().Is6() {
			return "", fmt.Errorf("%q is IPv6, only IPv4 sources are supported by the DOCKER-USER rules", s)
		}
		return p.Masked().String(), nil
	}
	a, err := netip.ParseAddr(s)
	if err != nil || !a.Is4() {
		return "", fmt.Errorf("%q is not a valid IPv4 address or CIDR", s)
	}
	return netip.PrefixFrom(a, 32).String(), nil
}

type ruleSpec struct {
	source string
	target string
}

func (m *Manager) specs(r Restriction) []ruleSpec {
	out := make([]ruleSpec, 0, len(r.Allow)+1)
	for _, a := range r.Allow {
		out = append(out, ruleSpec{a, targetReturn})
	}
	return append(out, ruleSpec{"", targetDrop})
}

func (m *Manager) insertArgs(r Restriction, s ruleSpec) []string {
	args := []string{"-w", "-I", dockerUserChain, "1", "-p", r.Protocol,
		"-m", "conntrack", "--ctorigdstport", strconv.Itoa(r.Port), "--ctdir", "ORIGINAL"}
	if s.source != "" {
		args = append(args, "-s", s.source)
	}
	return append(args, "-m", "comment", "--comment", m.tag(r.Protocol, r.Port), "-j", s.target)
}

// Plan renders the exact commands Apply would run, in execution order.
func (m *Manager) Plan(r Restriction) (Plan, error) {
	r, err := m.Normalize(r)
	if err != nil {
		return Plan{}, err
	}
	specs := m.specs(r)
	var cmds []string
	for i := len(specs) - 1; i >= 0; i-- {
		cmds = append(cmds, "iptables "+strings.Join(m.insertArgs(r, specs[i]), " "))
	}
	return Plan{
		Commands: cmds,
		Tag:      m.tag(r.Protocol, r.Port),
		Drops: fmt.Sprintf("New connections to published port %d/%s from any address except %s are dropped, including from the internet. Connections already allowed, other ports, and traffic from this host itself are unchanged.",
			r.Port, r.Protocol, strings.Join(r.Allow, ", ")),
		Persistence: "Held in the kernel only. The control plane stores the restriction and re-applies it on its reconcile loop and after a reboot or Docker restart while the control plane runs on this host.",
	}, nil
}

func (m *Manager) tagged(c Chain, tag string) []ChainRule {
	var out []ChainRule
	for _, r := range c.Rules {
		if r.Comment == tag {
			out = append(out, r)
		}
	}
	return out
}

func inSync(have []ChainRule, want []ruleSpec) bool {
	if len(have) != len(want) {
		return false
	}
	for i, h := range have {
		if h.Target != want[i].target || h.Source != want[i].source {
			return false
		}
	}
	return true
}

func (m *Manager) deleteRules(ctx context.Context, rules []ChainRule) error {
	for _, r := range rules {
		args := append([]string{"-w", "-D", dockerUserChain}, splitQuoted(r.Raw)...)
		if out, err := m.run(ctx, "iptables", args...); err != nil {
			return fmt.Errorf("exposure: delete rule %q: %s: %w", r.Raw, oneLine(out, err), err)
		}
	}
	return nil
}

// Apply installs the restriction idempotently and reports whether anything changed.
func (m *Manager) Apply(ctx context.Context, r Restriction) (changed bool, err error) {
	r, err = m.Normalize(r)
	if err != nil {
		return false, err
	}
	chain := m.ReadChain(ctx)
	if !chain.Readable {
		return false, fmt.Errorf("exposure: cannot apply: %s", chain.Reason)
	}
	have := m.tagged(chain, m.tag(r.Protocol, r.Port))
	want := m.specs(r)
	if inSync(have, want) {
		return false, nil
	}
	if err := m.deleteRules(ctx, have); err != nil {
		return true, err
	}
	for i := len(want) - 1; i >= 0; i-- {
		if out, err := m.run(ctx, "iptables", m.insertArgs(r, want[i])...); err != nil {
			_ = m.Remove(ctx, r.Port, r.Protocol)
			return true, fmt.Errorf("exposure: insert rule for %d/%s: %s: %w", r.Port, r.Protocol, oneLine(out, err), err)
		}
	}
	return true, nil
}

// Remove deletes the rules this manager tagged for a port, and nothing else.
func (m *Manager) Remove(ctx context.Context, port int, proto string) error {
	if proto == "" {
		proto = "tcp"
	}
	chain := m.ReadChain(ctx)
	if !chain.Readable {
		return fmt.Errorf("exposure: cannot remove: %s", chain.Reason)
	}
	return m.deleteRules(ctx, m.tagged(chain, m.tag(proto, port)))
}

// SyncResult counts what one Sync pass changed.
type SyncResult struct {
	Applied int
	Removed int
	Errors  []string
}

// Sync converges the chain to want: it applies every restriction and removes
// tagged rules whose restriction no longer exists.
func (m *Manager) Sync(ctx context.Context, want []Restriction) SyncResult {
	var res SyncResult
	chain := m.ReadChain(ctx)
	if !chain.Readable {
		res.Errors = append(res.Errors, chain.Reason)
		return res
	}
	wanted := map[string]bool{}
	for _, w := range want {
		n, err := m.Normalize(w)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%d/%s: %v", w.Port, w.Protocol, err))
			continue
		}
		wanted[m.tag(n.Protocol, n.Port)] = true
		changed, err := m.Apply(ctx, n)
		switch {
		case err != nil:
			res.Errors = append(res.Errors, fmt.Sprintf("%d/%s: %v", n.Port, n.Protocol, err))
		case changed:
			res.Applied++
		}
	}
	stale := map[string]bool{}
	for _, r := range chain.Rules {
		if strings.HasPrefix(r.Comment, m.prefix+tagInfix) && !wanted[r.Comment] {
			stale[r.Comment] = true
		}
	}
	for tag := range stale {
		proto, portStr, _ := strings.Cut(strings.TrimPrefix(tag, m.prefix+tagInfix), "/")
		port, err := strconv.Atoi(portStr)
		if err != nil {
			continue
		}
		if err := m.Remove(ctx, port, proto); err != nil {
			res.Errors = append(res.Errors, err.Error())
			continue
		}
		res.Removed++
	}
	return res
}
