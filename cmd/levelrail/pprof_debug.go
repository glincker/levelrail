package main

import (
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"time"
)

const envPprofAddr = "APP_PPROF_ADDR"

// startPprof serves heap and goroutine profiles on a loopback-only address when APP_PPROF_ADDR is set.
func startPprof(logger *slog.Logger) {
	addr := os.Getenv(envPprofAddr)
	if addr == "" {
		return
	}
	if host, _, err := net.SplitHostPort(addr); err != nil || (host != "127.0.0.1" && host != "localhost" && host != "::1") {
		logger.Warn("ignoring "+envPprofAddr+": must be a loopback host:port", slog.String("addr", addr))
		return
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Warn("pprof listener stopped", slog.String("error", err.Error()))
		}
	}()
}
