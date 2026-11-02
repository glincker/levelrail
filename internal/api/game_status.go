package api

import (
	"context"
	"net"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/gamestatus"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// How a game port is reachable, as reported in gameStatusResource.Exposure.
const (
	gameExposureStream    = "stream"
	gameExposureDirect    = "direct"
	gameExposurePublished = "published"
	gameExposurePrivate   = "private"
	gameExposureNone      = "none"
	gameExposureRemote    = "remote_node"
)

const (
	// gameStatusMinTimeout keeps a bad stored value from failing every probe.
	gameStatusMinTimeout = 200 * time.Millisecond
	// gameStatusCacheMaxEntries bounds the cache when apps come and go.
	gameStatusCacheMaxEntries = 1024
)

// gameStatusResource is the wire shape of GET /api/v1/apps/{name}/game-status.
type gameStatusResource struct {
	App string `json:"app"`
	// Exposure is stream, direct, published (public bind), private (reachable
	// only on this host), remote_node or none (no TCP port to probe).
	Exposure    string `json:"exposure"`
	Running     bool   `json:"running"`
	Online      bool   `json:"online"`
	Kind        string `json:"kind,omitempty"`
	Version     string `json:"version,omitempty"`
	Players     int    `json:"players"`
	MaxPlayers  int    `json:"max_players"`
	LatencyMS   int64  `json:"latency_ms"`
	Description string `json:"motd,omitempty"`
	// JoinAddress is the public host:port to give players; empty unless the
	// port is public and this server's public host is known.
	JoinAddress string `json:"join_address,omitempty"`
	JoinPort    int    `json:"join_port,omitempty"`
	CheckedAt   string `json:"checked_at"`
}

type gameStatusEntry struct {
	mu  sync.Mutex
	at  time.Time
	res gameStatusResource
}

// gameStatusCache reuses one answer per app so many viewers ping a game port at most once per TTL.
type gameStatusCache struct {
	mu      sync.Mutex
	entries map[string]*gameStatusEntry
}

func (c *gameStatusCache) entry(key string) *gameStatusEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = map[string]*gameStatusEntry{}
	}
	e, ok := c.entries[key]
	if !ok {
		if len(c.entries) >= gameStatusCacheMaxEntries {
			c.entries = map[string]*gameStatusEntry{}
		}
		e = &gameStatusEntry{}
		c.entries[key] = e
	}
	return e
}

// gameTarget is the port to probe on this host and how players reach it.
type gameTarget struct {
	exposure  string
	probePort int
	joinPort  int
}

// pickGameTarget chooses among the app's TCP ports: a stream first, then a
// public published port, a direct host port, and last a private published port.
func pickGameTarget(svc *store.DesiredService, streams []store.AppStream) gameTarget {
	var tcp []store.AppStream
	for _, s := range streams {
		if s.Protocol == "" || s.Protocol == store.AppStreamProtocolTCP {
			tcp = append(tcp, s)
		}
	}
	if len(tcp) > 0 {
		sort.Slice(tcp, func(i, j int) bool { return tcp[i].HostPort < tcp[j].HostPort })
		return gameTarget{exposure: gameExposureStream, probePort: tcp[0].HostPort, joinPort: tcp[0].HostPort}
	}
	var private *spec.PublishedPort
	for i, p := range svc.PublishedPorts {
		if p.EffectiveProtocol() != spec.PortProtocolTCP {
			continue
		}
		if p.EffectiveBind(svc.BindAddress) == spec.PortBindPublic {
			return gameTarget{exposure: gameExposurePublished, probePort: p.HostPort, joinPort: p.HostPort}
		}
		if private == nil {
			private = &svc.PublishedPorts[i]
		}
	}
	if isDirectExposure(svc) {
		return gameTarget{exposure: gameExposureDirect, probePort: *svc.HostPort, joinPort: *svc.HostPort}
	}
	if private != nil {
		return gameTarget{exposure: gameExposurePrivate, probePort: private.HostPort}
	}
	return gameTarget{exposure: gameExposureNone}
}

func (rt *Router) gameStatusTimeout(ctx context.Context) time.Duration {
	d := gameStatusMinTimeout
	if rt.runtimeSettings != nil {
		d = max(d, rt.runtimeSettings.Duration(ctx, runtimesettings.KeyGameStatusTimeout))
	}
	return d
}

func (rt *Router) gameStatusTTL(ctx context.Context) time.Duration {
	if rt.runtimeSettings == nil {
		return 0
	}
	return rt.runtimeSettings.Duration(ctx, runtimesettings.KeyGameStatusCache)
}

// handleGetAppGameStatus handles GET /api/v1/apps/{name}/game-status. It only
// ever dials loopback ports taken from the app's stored ports, never a caller value.
func (rt *Router) handleGetAppGameStatus(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.loadExposureApp(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	e := rt.gameStatus.entry(svc.Name)
	e.mu.Lock()
	defer e.mu.Unlock()
	if ttl := rt.gameStatusTTL(ctx); ttl > 0 && !e.at.IsZero() && time.Since(e.at) < ttl {
		writeJSON(w, http.StatusOK, e.res)
		return
	}
	res := rt.buildGameStatus(ctx, svc)
	e.at, e.res = time.Now(), res
	writeJSON(w, http.StatusOK, res)
}

func (rt *Router) buildGameStatus(ctx context.Context, svc *store.DesiredService) gameStatusResource {
	res := gameStatusResource{App: svc.Name, Exposure: gameExposureNone, CheckedAt: time.Now().UTC().Format(time.RFC3339)}
	if svc.NodeID != "" {
		res.Exposure = gameExposureRemote
		return res
	}
	streams, err := rt.appStreams.ListAppStreamsForService(ctx, svc.Name)
	if err != nil {
		streams = nil
	}
	t := pickGameTarget(svc, streams)
	res.Exposure = t.exposure
	if t.exposure == gameExposureNone || t.probePort < 1 {
		return res
	}
	_, res.Running = rt.observedHostPort(ctx, svc)
	if t.joinPort > 0 {
		res.JoinPort = t.joinPort
		if rt.publicHost != "" {
			res.JoinAddress = net.JoinHostPort(rt.publicHost, strconv.Itoa(t.joinPort))
		}
	}
	st, err := gamestatus.Probe(ctx, gamestatus.Dialer(rt.doctorDialContextOrDefault()),
		net.JoinHostPort("127.0.0.1", strconv.Itoa(t.probePort)), rt.gameStatusTimeout(ctx))
	if err != nil {
		return res
	}
	res.Online, res.Kind, res.Version = st.Online, st.Kind, st.Version
	res.Players, res.MaxPlayers, res.LatencyMS, res.Description = st.Players, st.MaxPlayers, st.LatencyMS, st.Description
	return res
}
