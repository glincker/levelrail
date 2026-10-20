package extdb

import (
	"context"
	"fmt"
	"net/netip"
	"strings"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

var builtinNetworks = map[string]bool{"bridge": true, "host": true, "none": true}

// findContainer looks a container up by exact name. It lists instead of
// inspecting by name because the local client scopes inspection to containers
// this platform created, and an adopted container is never one of those.
func findContainer(ctx context.Context, rt docker.Runtime, name string) (*docker.ContainerState, error) {
	states, err := rt.ListByPrefix(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("extdb: look up container %q: %w", name, err)
	}
	for i := range states {
		if states[i].Name == name {
			return &states[i], nil
		}
	}
	return nil, nil
}

// ResolveSource re-reads where an adopted container lives now. Containers get
// recreated, so the network and address recorded at adoption can be stale.
// It only reads container metadata; nothing is ever connected or changed.
// When the container cannot be found the connection is returned unchanged.
func ResolveSource(ctx context.Context, rt docker.Runtime, c Conn) Conn {
	if c.SourceContainer == "" || rt == nil {
		return c
	}
	st, err := findContainer(ctx, rt, c.SourceContainer)
	if err != nil || st == nil || !st.Running || len(st.Networks) == 0 {
		return c
	}
	var userDefined *docker.ContainerNetwork
	var bridge *docker.ContainerNetwork
	for i := range st.Networks {
		n := &st.Networks[i]
		switch {
		case n.Name == c.Network && !builtinNetworks[n.Name]:
			userDefined = n
		case n.Name == "bridge":
			bridge = n
		case !builtinNetworks[n.Name] && userDefined == nil:
			userDefined = n
		}
	}
	_, hostIsIP := parseHostIP(c.Host)
	switch {
	case userDefined != nil:
		c.Network = userDefined.Name
		if hostIsIP || c.Host == c.SourceContainer {
			c.Host = st.Name
		}
	case bridge != nil && hostIsIP:
		c.Host = bridge.IPAddress
	}
	return c
}

func parseHostIP(host string) (netip.Addr, bool) {
	a, err := netip.ParseAddr(strings.Trim(host, "[]"))
	return a, err == nil
}

// Refresh applies ResolveSource to a stored record and reports whether its
// host or network changed.
func Refresh(ctx context.Context, rt docker.Runtime, rec store.ExternalDatabase) (store.ExternalDatabase, bool) {
	c := ResolveSource(ctx, rt, Conn{Host: rec.Host, Network: rec.Network, SourceContainer: rec.SourceContainer})
	if c.Host == rec.Host && c.Network == rec.Network {
		return rec, false
	}
	rec.Host, rec.Network = c.Host, c.Network
	return rec, true
}
