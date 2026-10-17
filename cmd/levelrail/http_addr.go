package main

import (
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"
)

const loopbackHTTPAddr = "127.0.0.1:8080"

// runningInContainer is a variable so tests can pick either install shape.
var runningInContainer = func() bool {
	for _, marker := range []string{"/.dockerenv", "/run/.containerenv"} {
		if _, err := os.Stat(marker); err == nil {
			return true
		}
	}
	return false
}

// Loopback is the default only outside a container with an HTTPS dashboard URL.
// Any other loopback default would lock the operator out of first sign-in.
func resolveHTTPAddr(getenv func(string) string, inContainer bool) string {
	if v := getenv("APP_HTTP_ADDR"); v != "" {
		return v
	}
	url := strings.ToLower(strings.TrimSpace(getenv("APP_DASHBOARD_URL")))
	if !inContainer && strings.HasPrefix(url, "https://") {
		return loopbackHTTPAddr
	}
	return defaultHTTPAddr
}

// httpAddrExposed reports whether addr listens beyond this host.
func httpAddrExposed(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	switch {
	case host == "":
		return true
	case strings.EqualFold(host, "localhost"):
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return !ip.IsLoopback()
	}
	return true
}

// httpAddrWarner logs the exposed-listener warning at most once.
type httpAddrWarner struct{ once sync.Once }

func (w *httpAddrWarner) warn(logger *slog.Logger, addr string, inContainer bool) {
	if inContainer || !httpAddrExposed(addr) {
		return
	}
	w.once.Do(func() {
		logger.Warn("dashboard listener is plain HTTP on a non-loopback address: bind 127.0.0.1 and front it with the ingress over HTTPS",
			slog.String("addr", addr), slog.String("override", "APP_HTTP_ADDR"))
	})
}

var exposedHTTPAddrWarning httpAddrWarner
