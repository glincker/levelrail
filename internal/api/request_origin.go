package api

import (
	"context"
	"net/http"
	"strings"
)

type requestOriginKey struct{}

// withRequestOrigin records the caller-visible origin of r, the fallback
// for absolute URLs when no primary domain is configured.
func withRequestOrigin(ctx context.Context, r *http.Request) context.Context {
	host := r.Host
	if host == "" || strings.ContainsAny(host, "/ \t\r\n") {
		return ctx
	}
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return context.WithValue(ctx, requestOriginKey{}, scheme+"://"+host)
}

func requestOriginFrom(ctx context.Context) string {
	s, _ := ctx.Value(requestOriginKey{}).(string)
	return s
}
