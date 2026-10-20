package ingress

import (
	"sort"
	"strings"
)

// ServedHosts lists every hostname cfg routes or manages a certificate for.
func ServedHosts(cfg *Config) []string {
	if cfg == nil {
		return nil
	}
	set := map[string]bool{}
	add := func(h string) {
		if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
			set[h] = true
		}
	}
	if cfg.Apps.HTTP != nil {
		for _, srv := range cfg.Apps.HTTP.Servers {
			if srv == nil {
				continue
			}
			for _, r := range srv.Routes {
				for _, m := range r.Match {
					for _, h := range m.Host {
						add(h)
					}
				}
			}
		}
	}
	if cfg.Apps.TLS != nil && cfg.Apps.TLS.Automation != nil {
		for _, p := range cfg.Apps.TLS.Automation.Policies {
			for _, s := range p.Subjects {
				add(s)
			}
		}
	}
	out := make([]string, 0, len(set))
	for h := range set {
		out = append(out, h)
	}
	sort.Strings(out)
	return out
}
