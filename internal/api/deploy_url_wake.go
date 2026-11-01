package api

import (
	"context"
	"io"
	"net/http"
	"strings"
)

type deployURLOwner interface {
	DeployURLApp(ctx context.Context, serviceName string) (string, bool, error)
}

// isDeployURLCopy reports whether name is a retained deploy-URL copy.
func (rt *Router) isDeployURLCopy(ctx context.Context, name string) bool {
	o, ok := rt.deployAttempts.(deployURLOwner)
	if !ok {
		return false
	}
	_, found, err := o.DeployURLApp(ctx, name)
	return err == nil && found
}

// looksLikeBrowserVisit tells a person opening the address from a crawler or
// scanner. Browsers send Sec-Fetch-Mode: navigate on a top-level visit; over
// plain HTTP they omit it, so an HTML Accept from a non-bot agent counts there.
func looksLikeBrowserVisit(r *http.Request) bool {
	if containsAny(strings.ToLower(r.UserAgent()), "bot", "crawl", "spider", "slurp", "scan", "headless", "curl", "python", "go-http") {
		return false
	}
	if mode := r.Header.Get("Sec-Fetch-Mode"); mode != "" {
		return mode == "navigate"
	}
	return r.Header.Get("X-Forwarded-Proto") == "http" && strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/html")
}

// writeSleepingNoWake answers an automated request to a sleeping copy without
// starting it, so a scanner finding a new hostname costs nothing.
func writeSleepingNoWake(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Content-Type", "text/plain; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Robots-Tag", "noindex, nofollow")
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = io.WriteString(w, "This deploy is asleep. Open it in a browser to wake it.\n")
}
