package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	envAppTunnelEnabled     = "APP_TUNNEL_ENABLED"
	envAppTunnelIdleTimeout = "APP_TUNNEL_IDLE_TIMEOUT"
	envAppTunnelMaxPerUser  = "APP_TUNNEL_MAX_PER_USER"

	appTunnelDefaultIdle      = 15 * time.Minute
	appTunnelDefaultMaxPerUsr = 5
	appTunnelDialTimeout      = 5 * time.Second
)

// appTunnelSlots counts open tunnels per caller so one account cannot hold unbounded sockets.
var appTunnelSlots = struct {
	mu sync.Mutex
	n  map[string]int
}{n: map[string]int{}}

func acquireAppTunnelSlot(user string, limit int) bool {
	appTunnelSlots.mu.Lock()
	defer appTunnelSlots.mu.Unlock()
	if appTunnelSlots.n[user] >= limit {
		return false
	}
	appTunnelSlots.n[user]++
	return true
}

func releaseAppTunnelSlot(user string) {
	appTunnelSlots.mu.Lock()
	defer appTunnelSlots.mu.Unlock()
	if appTunnelSlots.n[user] <= 1 {
		delete(appTunnelSlots.n, user)
		return
	}
	appTunnelSlots.n[user]--
}

func appTunnelEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(envAppTunnelEnabled))) {
	case "0", "false", "off", "no":
		return false
	}
	return true
}

// resolveAppTunnelTarget picks the one service a tunnel may reach. A name that is a
// service stands for itself only; a name that is an app group id may pick a member.
// Siblings of a service are never reachable through that service's own name, so a
// policy scoped to one service cannot be used to reach another.
func (rt *Router) resolveAppTunnelTarget(ctx context.Context, name, service string) (*store.DesiredService, int, string) {
	svc, err := rt.apps.GetDesiredService(ctx, name)
	if err == nil {
		if service != "" && service != svc.Name {
			return nil, http.StatusNotFound, "service not found in this app"
		}
		return svc, 0, ""
	}
	if !errors.Is(err, store.ErrServiceNotFound) {
		return nil, http.StatusInternalServerError, "internal error"
	}
	if rt.appGroups == nil || service == "" {
		return nil, http.StatusNotFound, "app not found"
	}
	members, err := rt.appGroups.ListServicesByApp(ctx, name)
	if err != nil {
		return nil, http.StatusInternalServerError, "internal error"
	}
	for i := range members {
		m := members[i]
		if m.AppID == name && (m.Name == service || m.Name == name+"-"+service) {
			return &m, 0, ""
		}
	}
	return nil, http.StatusNotFound, "service not found in this app"
}

// appTunnelPortAllowed reports whether port is one the service spec declares: its routed
// port, a published container port, or an EXPOSE of the image.
func appTunnelPortAllowed(svc *store.DesiredService, port int, imageExposed []string) bool {
	if port <= 0 || port > 65535 {
		return false
	}
	if svc.Port == port {
		return true
	}
	for _, p := range svc.PublishedPorts {
		if p.ContainerPort == port && p.EffectiveProtocol() == spec.PortProtocolTCP {
			return true
		}
	}
	for _, e := range imageExposed {
		num, proto, _ := strings.Cut(e, "/")
		if n, err := strconv.Atoi(num); err == nil && n == port && (proto == "" || proto == spec.PortProtocolTCP) {
			return true
		}
	}
	return false
}

// appTunnelPorts lists the tcp container ports a tunnel to svc may use.
func appTunnelPorts(svc *store.DesiredService, imageExposed []string) []int {
	seen := map[int]bool{}
	var out []int
	add := func(p int) {
		if p > 0 && p <= 65535 && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	add(svc.Port)
	for _, p := range svc.PublishedPorts {
		if p.EffectiveProtocol() == spec.PortProtocolTCP {
			add(p.ContainerPort)
		}
	}
	for _, e := range imageExposed {
		num, proto, _ := strings.Cut(e, "/")
		if n, err := strconv.Atoi(num); err == nil && (proto == "" || proto == spec.PortProtocolTCP) {
			add(n)
		}
	}
	return out
}

func (rt *Router) imageExposedPorts(ctx context.Context, nodeRuntime docker.Runtime, image string) []string {
	insp, ok := nodeRuntime.(docker.ImageDetailInspector)
	if !ok || image == "" {
		return nil
	}
	d, err := insp.InspectImageDetails(ctx, image)
	if err != nil || d == nil {
		return nil
	}
	return d.ExposedPorts
}

// handleAppTunnel handles GET /api/v1/apps/{name}/tunnel?service=&port=: a websocket
// relayed to one declared port of an app container on the platform network. Like the
// database tunnel it reaches only containers on the control plane node.
func (rt *Router) handleAppTunnel(w http.ResponseWriter, r *http.Request) {
	if !appTunnelEnabled() {
		writeError(w, http.StatusForbidden, "app tunnels are disabled on this control plane")
		return
	}
	if rt.execRuntime == nil {
		writeError(w, http.StatusNotImplemented, "app tunnels are not configured on this control plane")
		return
	}
	name := r.PathValue("name")
	target, code, msg := rt.resolveAppTunnelTarget(r.Context(), name, r.URL.Query().Get("service"))
	if target == nil {
		if code == http.StatusInternalServerError {
			rt.logger.Error("api: app tunnel: resolve target failed", slog.String("name", name))
		}
		writeError(w, code, msg)
		return
	}
	if target.NodeID != "" && !rt.isLocalNode(target.NodeID) {
		writeError(w, http.StatusConflict, "this app runs on another node, tunnels reach only apps on the control plane node")
		return
	}
	nodeRuntime, err := rt.execRuntime(target.NodeID)
	if err != nil {
		writeError(w, http.StatusBadGateway, "app's node is not currently reachable")
		return
	}
	inspectCtx, cancelInspect := context.WithTimeout(r.Context(), dockerInspectTimeout)
	state, err := nodeRuntime.InspectByName(inspectCtx, application.ContainerName(target.Name, application.NameImage(*target), target.RestartNonce))
	exposed := rt.imageExposedPorts(inspectCtx, nodeRuntime, target.Image)
	cancelInspect()
	if err != nil {
		rt.internalError(w, "api: app tunnel: inspect container failed", err, slog.String("name", target.Name))
		return
	}
	if state == nil || !state.Running {
		writeError(w, http.StatusConflict, "app has no running container")
		return
	}

	port := target.Port
	if raw := r.URL.Query().Get("port"); raw != "" {
		p, perr := strconv.Atoi(raw)
		if perr != nil {
			writeError(w, http.StatusBadRequest, "port must be a number")
			return
		}
		port = p
	}
	if !appTunnelPortAllowed(target, port, exposed) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("port %d is not exposed by service %q, allowed: %v", port, target.Name, appTunnelPorts(target, exposed)))
		return
	}
	ip := ""
	for _, n := range state.Networks {
		if n.IPAddress != "" {
			ip = n.IPAddress
			break
		}
	}
	if ip == "" {
		writeError(w, http.StatusConflict, "the app container has no reachable address")
		return
	}

	user := rt.accessActor(r).id
	if !acquireAppTunnelSlot(user, envInt(envAppTunnelMaxPerUser, appTunnelDefaultMaxPerUsr)) {
		writeError(w, http.StatusTooManyRequests, "too many open app tunnels for this account")
		return
	}
	defer releaseAppTunnelSlot(user)

	upstream, err := (&net.Dialer{Timeout: appTunnelDialTimeout}).DialContext(r.Context(), "tcp", net.JoinHostPort(ip, strconv.Itoa(port)))
	if err != nil {
		rt.logger.Warn("api: app tunnel: dial failed", slog.String("name", target.Name), slog.String("error", err.Error()))
		writeError(w, http.StatusBadGateway, "could not reach the app port")
		return
	}
	defer func() { _ = upstream.Close() }()

	ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		return
	}
	ws.SetReadLimit(1 << 20)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	client := websocket.NetConn(ctx, ws, websocket.MessageBinary)
	defer func() { _ = client.Close() }()

	detail := fmt.Sprintf("service=%s,port=%d", target.Name, port)
	rt.auditAppTunnel(r, "app.tunnel.open", name, detail, http.StatusSwitchingProtocols)
	in, out := relayAppTunnel(ctx, cancel, client, upstream, envDuration(envAppTunnelIdleTimeout, appTunnelDefaultIdle))
	rt.auditAppTunnel(r, "app.tunnel.close", name, fmt.Sprintf("%s,bytes_in=%d,bytes_out=%d", detail, in, out), http.StatusOK)
}

// relayAppTunnel copies both ways until either side ends or no byte moves for idle,
// and reports bytes sent toward the app and bytes returned.
func relayAppTunnel(ctx context.Context, cancel context.CancelFunc, client, upstream net.Conn, idle time.Duration) (in, out int64) {
	var last atomic.Int64
	last.Store(time.Now().UnixNano())
	var bin, bout atomic.Int64
	copyCount := func(dst io.Writer, src io.Reader, n *atomic.Int64) {
		buf := make([]byte, 32*1024)
		for {
			c, err := src.Read(buf)
			if c > 0 {
				last.Store(time.Now().UnixNano())
				n.Add(int64(c))
				if _, werr := dst.Write(buf[:c]); werr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}
	done := make(chan struct{}, 2)
	go func() { copyCount(upstream, client, &bin); done <- struct{}{} }()
	go func() { copyCount(client, upstream, &bout); done <- struct{}{} }()

	tick := idle / 4
	if tick < 10*time.Millisecond {
		tick = 10 * time.Millisecond
	}
	t := time.NewTicker(tick)
	defer t.Stop()
	for {
		select {
		case <-done:
			cancel()
			_ = upstream.Close()
			_ = client.Close()
			return bin.Load(), bout.Load()
		case <-ctx.Done():
			_ = upstream.Close()
			_ = client.Close()
			return bin.Load(), bout.Load()
		case now := <-t.C:
			if now.UnixNano()-last.Load() > idle.Nanoseconds() {
				cancel()
				_ = upstream.Close()
				_ = client.Close()
				return bin.Load(), bout.Load()
			}
		}
	}
}

func (rt *Router) auditAppTunnel(r *http.Request, action, app, detail string, status int) {
	a := rt.accessActor(r)
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: app tunnel audit id failed", slog.String("error", err.Error()))
		return
	}
	entry := store.AuditEntry{
		ID: id, ActorType: a.kind, ActorID: a.id, ActorName: a.name,
		Ability: AbilityRoot, Method: action, Path: "/api/v1/apps/" + app + "/tunnel#" + detail, StatusCode: status,
		RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
	}
	if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(r.Context()), entry); err != nil {
		rt.logger.Warn("api: app tunnel audit save failed", slog.String("error", err.Error()), slog.String("app", app))
	}
}
