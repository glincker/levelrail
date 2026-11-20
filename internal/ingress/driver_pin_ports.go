package ingress

import (
	"net"
	"strconv"
	"strings"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

// Caddy binds with SO_REUSEPORT, so every reload of an address ending in :0
// opens a fresh ephemeral port. Pinning the first bound port keeps links stable.
func (d *Driver) pinEphemeralListens(cfg *Config) {
	if cfg.Apps.HTTP == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for name, srv := range cfg.Apps.HTTP.Servers {
		if srv == nil {
			continue
		}
		listen := append([]string(nil), srv.Listen...)
		for i, addr := range listen {
			if pinned, ok := d.pinnedListens[pinKey(name, i, addr)]; ok {
				listen[i] = pinned
			}
		}
		srv.Listen = listen
	}
}

// recordEphemeralListens remembers the concrete port of each :0 listener once
// the config is live.
func (d *Driver) recordEphemeralListens(cfg *Config) {
	if cfg.Apps.HTTP == nil {
		return
	}
	app, err := caddy.ActiveContext().AppIfConfigured("http")
	if err != nil {
		return
	}
	httpApp, ok := app.(*caddyhttp.App)
	if !ok {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for name, srv := range cfg.Apps.HTTP.Servers {
		live := httpApp.Servers[name]
		if srv == nil || live == nil {
			continue
		}
		lns := live.Listeners()
		if len(lns) != len(srv.Listen) {
			continue
		}
		for i, addr := range srv.Listen {
			if !isEphemeralListen(addr) {
				continue
			}
			port := portOfAddr(lns[i].Addr())
			if port == 0 {
				continue
			}
			host := addr[:strings.LastIndex(addr, ":")]
			if d.pinnedListens == nil {
				d.pinnedListens = map[string]string{}
			}
			d.pinnedListens[pinKey(name, i, addr)] = net.JoinHostPort(strings.Trim(host, "[]"), strconv.Itoa(port))
		}
	}
}

func isEphemeralListen(addr string) bool {
	i := strings.LastIndex(addr, ":")
	return i >= 0 && addr[i+1:] == "0" && !strings.HasPrefix(addr, "fd/")
}

func pinKey(server string, idx int, addr string) string {
	return server + "|" + strconv.Itoa(idx) + "|" + addr
}
