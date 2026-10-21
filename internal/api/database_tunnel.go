package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/coder/websocket"

	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/store"
)

const databaseTunnelDialTimeout = 5 * time.Second

// handleDatabaseTunnel handles GET /api/v1/databases/{name}/tunnel: a websocket relayed to the database's own port.
// Only databases on the control plane's node are reachable, so no node needs an inbound port.
func (rt *Router) handleDatabaseTunnel(w http.ResponseWriter, r *http.Request) {
	d, err := rt.databases.GetDesiredDatabase(r.Context(), r.PathValue("name"))
	if errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: database tunnel: load database failed", err, slog.String("name", r.PathValue("name")))
		return
	}
	if d.NodeID != "" && !rt.isLocalNode(d.NodeID) {
		writeError(w, http.StatusConflict, "this database runs on another node, tunnels reach only databases on the control plane node")
		return
	}
	addr, err := rt.databaseTunnelAddr(r.Context(), d)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	upstream, err := (&net.Dialer{Timeout: databaseTunnelDialTimeout}).DialContext(r.Context(), "tcp", addr)
	if err != nil {
		rt.logger.Warn("api: database tunnel: dial failed", slog.String("name", d.Name), slog.String("error", err.Error()))
		writeError(w, http.StatusBadGateway, "could not reach the database")
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

	rt.auditDatabaseAccess(r, "database.tunnel.open", d.Name, "", http.StatusSwitchingProtocols)
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(upstream, client); done <- struct{}{} }()
	go func() { _, _ = io.Copy(client, upstream); done <- struct{}{} }()
	<-done
}

func (rt *Router) databaseTunnelAddr(ctx context.Context, d *store.DesiredDatabase) (string, error) {
	state := rt.databaseContainerState(ctx, d)
	if state == nil || !state.Running {
		return "", errors.New("the database is not running")
	}
	ip := ""
	for _, n := range state.Networks {
		if n.IPAddress != "" {
			ip = n.IPAddress
			break
		}
	}
	if ip == "" {
		return "", errors.New("the database container has no reachable address")
	}
	port, ok := database.ContainerPort(d.Engine)
	if !ok {
		return "", fmt.Errorf("engine %q has no known port", d.Engine)
	}
	if tlsPort, ok := database.TLSContainerPort(d.Engine); ok && rt.databaseTLSEnabled(ctx, *d) {
		port = tlsPort
	}
	return net.JoinHostPort(ip, strconv.Itoa(port)), nil
}
