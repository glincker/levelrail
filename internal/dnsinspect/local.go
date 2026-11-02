package dnsinspect

import (
	"context"
	"net"
	"os"
	"strings"

	"github.com/miekg/dns"
)

// LocalName labels this host's own resolver in reports.
const LocalName = "this server"

const resolvConf = "/etc/resolv.conf"

// systemdStub is the address systemd-resolved listens on; its cache can be
// flushed with resolvectl.
const systemdStub = "127.0.0.53"

// localServers prefers override, else the servers in resolv.conf.
func localServers(override []string) []string {
	if len(override) > 0 {
		return override
	}
	cfg, err := dns.ClientConfigFromFile(resolvConf)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(cfg.Servers))
	for _, s := range cfg.Servers {
		out = append(out, net.JoinHostPort(s, cfg.Port))
	}
	return out
}

// Local asks this host's configured resolver directly, so the answer carries
// the cache's remaining TTL that net.Resolver hides.
func (in *Inspector) Local(ctx context.Context, host string) Source {
	list := localServers(in.LocalServers)
	if len(list) == 0 {
		return Source{Name: LocalName, Error: "no resolver configured"}
	}
	return in.lookup(ctx, LocalName, list[0], strings.TrimSuffix(host, "."), true)
}

// FlushCommand is the one-line command that clears this host's resolver
// cache, or empty when the resolver is not one we can name. It is shown to
// the operator, never run.
func FlushCommand() string {
	for _, s := range localServers(nil) {
		if h, _, err := net.SplitHostPort(s); err == nil && h == systemdStub {
			return "resolvectl flush-caches"
		}
	}
	if _, err := os.Stat("/run/systemd/resolve/stub-resolv.conf"); err == nil {
		return "resolvectl flush-caches"
	}
	return ""
}
