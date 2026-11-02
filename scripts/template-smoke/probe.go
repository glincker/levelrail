package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/catalog"
)

const (
	probeInterval = 3 * time.Second
	probeDialWait = 3 * time.Second
)

// probeVerdict decides whether one observation of the declared health path
// counts as healthy. HTTP passes on 200 to 399 or the declared status, a tcp
// kind passes on a successful connect.
func probeVerdict(h catalog.ManifestHealthcheck, status int, connected bool) (bool, string) {
	if strings.EqualFold(h.Kind, "tcp") {
		if connected {
			return true, "tcp port accepts connections"
		}
		return false, "tcp port not reachable"
	}
	switch {
	case status == 0:
		return false, "health path did not answer"
	case status >= 200 && status < 400:
		return true, fmt.Sprintf("health path answered %d", status)
	case h.Expect != 0 && status == h.Expect:
		return true, fmt.Sprintf("health path answered the declared %d", status)
	}
	return false, fmt.Sprintf("health path answered %d", status)
}

// publishedProbe is the compose override line that exposes the health port
// on an ephemeral loopback port, so the host can probe it on any engine.
func publishedProbe(port int) string {
	return fmt.Sprintf("    ports: !override [\"127.0.0.1::%d\"]\n", port)
}

// probeHealth publishes nothing itself: the override already did. It retries
// until wait runs out, then reports the last observation.
func probeHealth(ctx context.Context, p project, h catalog.ManifestHealthcheck, wait time.Duration) (status int, ok bool, detail string) {
	out, err := p.run(ctx, "port", h.Service, strconv.Itoa(h.Port))
	addr := strings.TrimSpace(out)
	if err != nil || addr == "" {
		return 0, false, "health port is not published"
	}
	if i := strings.LastIndex(addr, "\n"); i >= 0 {
		addr = strings.TrimSpace(addr[:i])
	}
	deadline := time.Now().Add(wait)
	for {
		status, connected := observe(ctx, addr, h)
		if ok, detail = probeVerdict(h, status, connected); ok || time.Now().After(deadline) || ctx.Err() != nil {
			return status, ok, detail
		}
		select {
		case <-ctx.Done():
		case <-time.After(probeInterval):
		}
	}
}

func observe(ctx context.Context, addr string, h catalog.ManifestHealthcheck) (int, bool) {
	if strings.EqualFold(h.Kind, "tcp") {
		c, err := (&net.Dialer{Timeout: probeDialWait}).DialContext(ctx, "tcp", addr)
		if err != nil {
			return 0, false
		}
		_ = c.Close()
		return 0, true
	}
	path := h.Path
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+path, nil)
	if err != nil {
		return 0, false
	}
	// Redirects are a pass on their own; following them could leave the host.
	cl := &http.Client{Timeout: probeDialWait * 2, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := cl.Do(req) //nolint:gosec // loopback address from docker compose port
	if err != nil {
		return 0, false
	}
	_ = resp.Body.Close()
	return resp.StatusCode, false
}
