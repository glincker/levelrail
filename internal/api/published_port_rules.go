package api

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// publishedPortRuleLabelPrefix tags the allow rules owned by per-port public binds.
const publishedPortRuleLabelPrefix = "published-port:"

func publishedPortRuleLabel(service string, p store.PublishedPort) string {
	return publishedPortRuleLabelPrefix + service + ":" + strconv.Itoa(p.HostPort) + "/" + p.EffectiveProtocol()
}

// syncPublishedPortRules keeps exactly one allow rule per published port of
// svc whose own bind is public, and removes tagged rules for any other port.
func (rt *Router) syncPublishedPortRules(ctx context.Context, svc store.DesiredService) error {
	want := map[string]store.PublishedPort{}
	for _, p := range svc.PublishedPorts {
		if p.Bind == spec.PortBindPublic && p.ContainerPort != svc.Port {
			want[publishedPortRuleLabel(svc.Name, p)] = p
		}
	}
	rules, err := rt.firewallRules.ListFirewallRules(ctx)
	if err != nil {
		return fmt.Errorf("list firewall rules: %w", err)
	}
	prefix := publishedPortRuleLabelPrefix + svc.Name + ":"
	for _, r := range rules {
		if !strings.HasPrefix(r.Label, prefix) {
			continue
		}
		if _, keep := want[r.Label]; keep {
			delete(want, r.Label)
			continue
		}
		if err := rt.firewallRules.DeleteFirewallRule(ctx, r.ID); err != nil && !errors.Is(err, store.ErrFirewallRuleNotFound) {
			return fmt.Errorf("remove firewall rule %s: %w", r.Label, err)
		}
	}
	for label, p := range want {
		id, err := randomFirewallRuleID()
		if err != nil {
			return fmt.Errorf("firewall rule id: %w", err)
		}
		rule := store.FirewallRule{ID: id, Port: p.HostPort, Protocol: p.EffectiveProtocol(), Action: store.FirewallRuleActionAllow,
			Label: label, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
		if err := rt.firewallRules.SaveFirewallRule(ctx, rule); err != nil {
			return fmt.Errorf("save firewall rule %s: %w", label, err)
		}
	}
	return nil
}
