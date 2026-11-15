package hostfw

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// dnsPort is the only port the DNS rules ever open: Docker forwards to port 53 of a configured server.
const dnsPort = 53

var (
	// ErrNoDNSTool means neither iptables nor nft is on the host.
	ErrNoDNSTool = errors.New("hostfw: no iptables or nft found")
	// ErrBadInterface means a bridge name failed validation and was not passed to any tool.
	ErrBadInterface = errors.New("hostfw: invalid interface name")

	ifaceRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,14}$`)
	tagRe   = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,40}$`)
	nftRe   = regexp.MustCompile(`iifname "([A-Za-z0-9_.-]+)" (udp|tcp) dport 53 accept comment "([^"]+)" # handle (\d+)`)
)

// DNSRules keeps an INPUT accept for udp and tcp 53 from the managed bridges, tagged so it can be removed.
type DNSRules struct {
	Deps Deps
	// Tag is the comment on every rule this type owns; rules without it are never touched.
	Tag string
}

type dnsRuleKey struct{ iface, proto string }

// Backend reports which tool the rules go through: iptables first, then nft.
func (r DNSRules) Backend() Backend {
	if _, err := r.Deps.LookPath("iptables"); err == nil {
		return BackendIptables
	}
	if _, err := r.Deps.LookPath("nft"); err == nil {
		return BackendNftables
	}
	return BackendNone
}

func (r DNSRules) check(ifaces []string) error {
	if !tagRe.MatchString(r.Tag) {
		return fmt.Errorf("hostfw: invalid rule tag %q", r.Tag)
	}
	for _, n := range ifaces {
		if !ifaceRe.MatchString(n) {
			return fmt.Errorf("%w: %q", ErrBadInterface, n)
		}
	}
	return nil
}

// Ensure makes exactly the wanted interfaces accept udp and tcp 53, and drops tagged rules for others.
func (r DNSRules) Ensure(ctx context.Context, ifaces []string) error {
	if err := r.check(ifaces); err != nil {
		return err
	}
	want := map[dnsRuleKey]bool{}
	for _, n := range ifaces {
		want[dnsRuleKey{n, "udp"}] = true
		want[dnsRuleKey{n, "tcp"}] = true
	}
	have, err := r.listTagged(ctx)
	if err != nil {
		return err
	}
	for k := range have {
		if !want[k] {
			if err := r.del(ctx, k, have[k]); err != nil {
				return err
			}
		}
	}
	keys := make([]dnsRuleKey, 0, len(want))
	for k := range want {
		if _, ok := have[k]; !ok {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].iface+keys[i].proto < keys[j].iface+keys[j].proto })
	for _, k := range keys {
		if err := r.add(ctx, k); err != nil {
			return err
		}
	}
	return nil
}

// Present reports, per interface, whether both the udp and tcp rule exist.
func (r DNSRules) Present(ctx context.Context, ifaces []string) (map[string]bool, error) {
	if err := r.check(ifaces); err != nil {
		return nil, err
	}
	have, err := r.listTagged(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, n := range ifaces {
		_, u := have[dnsRuleKey{n, "udp"}]
		_, t := have[dnsRuleKey{n, "tcp"}]
		out[n] = u && t
	}
	return out, nil
}

// Remove deletes every tagged rule. Safe to call when none exist.
func (r DNSRules) Remove(ctx context.Context) error {
	if !tagRe.MatchString(r.Tag) {
		return fmt.Errorf("hostfw: invalid rule tag %q", r.Tag)
	}
	have, err := r.listTagged(ctx)
	if err != nil {
		if errors.Is(err, ErrNoDNSTool) {
			return nil
		}
		return err
	}
	for k, h := range have {
		if err := r.del(ctx, k, h); err != nil {
			return err
		}
	}
	return nil
}

func (r DNSRules) iptArgs(op string, k dnsRuleKey) []string {
	args := []string{"-w", op, "INPUT"}
	if op == "-I" {
		args = append(args, "1")
	}
	return append(args, "-i", k.iface, "-p", k.proto, "--dport", strconv.Itoa(dnsPort),
		"-m", "comment", "--comment", r.Tag, "-j", "ACCEPT")
}

// listTagged returns the tagged rules; the value is the nft handle (empty for iptables).
func (r DNSRules) listTagged(ctx context.Context) (map[dnsRuleKey]string, error) {
	out := map[dnsRuleKey]string{}
	switch r.Backend() {
	case BackendIptables:
		raw, err := r.Deps.Run(ctx, "iptables", "-w", "-S", "INPUT")
		if err != nil {
			return nil, fmt.Errorf("hostfw: list INPUT rules: %w", err)
		}
		for _, line := range strings.Split(string(raw), "\n") {
			f := strings.Fields(line)
			var iface, proto, tag string
			for i := 0; i+1 < len(f); i++ {
				switch f[i] {
				case "-i":
					iface = f[i+1]
				case "-p":
					proto = f[i+1]
				case "--comment":
					tag = strings.Trim(f[i+1], `"`)
				}
			}
			if tag == r.Tag && ifaceRe.MatchString(iface) && (proto == "udp" || proto == "tcp") {
				out[dnsRuleKey{iface, proto}] = ""
			}
		}
	case BackendNftables:
		raw, err := r.Deps.Run(ctx, "nft", "-a", "list", "chain", "inet", "filter", "input")
		if err != nil {
			return nil, fmt.Errorf("hostfw: list nft input chain (only an inet filter input chain is managed): %w", err)
		}
		for _, m := range nftRe.FindAllStringSubmatch(string(raw), -1) {
			if m[3] == r.Tag {
				out[dnsRuleKey{m[1], m[2]}] = m[4]
			}
		}
	default:
		return nil, ErrNoDNSTool
	}
	return out, nil
}

func (r DNSRules) add(ctx context.Context, k dnsRuleKey) error {
	if r.Backend() == BackendNftables {
		_, err := r.Deps.Run(ctx, "nft", "insert", "rule", "inet", "filter", "input", "iifname", k.iface,
			k.proto, "dport", strconv.Itoa(dnsPort), "accept", "comment", r.Tag)
		return wrapRule("add", k, err)
	}
	if _, err := r.Deps.Run(ctx, "iptables", r.iptArgs("-C", k)...); err == nil {
		return nil
	}
	_, err := r.Deps.Run(ctx, "iptables", r.iptArgs("-I", k)...)
	return wrapRule("add", k, err)
}

func (r DNSRules) del(ctx context.Context, k dnsRuleKey, handle string) error {
	if r.Backend() == BackendNftables {
		if _, err := strconv.Atoi(handle); err != nil {
			return fmt.Errorf("hostfw: bad nft handle %q", handle)
		}
		_, err := r.Deps.Run(ctx, "nft", "delete", "rule", "inet", "filter", "input", "handle", handle)
		return wrapRule("remove", k, err)
	}
	_, err := r.Deps.Run(ctx, "iptables", r.iptArgs("-D", k)...)
	return wrapRule("remove", k, err)
}

func wrapRule(verb string, k dnsRuleKey, err error) error {
	if err != nil {
		return fmt.Errorf("hostfw: %s dns accept for %s %s: %w", verb, k.iface, k.proto, err)
	}
	return nil
}
