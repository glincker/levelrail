package api

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/glincker/levelrail/internal/ingresstrust"
)

const envInsecureLoginLogInterval = "APP_INSECURE_LOGIN_LOG_INTERVAL" //nolint:gosec // env var name

type ingressHopKey struct{}

// ingressHop is what the hop middleware learned about a request.
type ingressHop struct {
	verified   bool   // carried this process's ingress marker
	markerSeen bool   // carried a marker header, valid or not
	proto      string // X-Forwarded-Proto as received, for refusal logs
}

func ingressHopOf(r *http.Request) ingressHop {
	h, _ := r.Context().Value(ingressHopKey{}).(ingressHop)
	return h
}

func requestFromIngress(r *http.Request) bool { return ingressHopOf(r).verified }

// IngressHopMiddleware is the only place the ingress marker is honoured. It
// always removes the marker header, records whether it was valid, and drops
// X-Forwarded-Proto and X-Forwarded-Host from any request whose peer is not
// trusted, so no handler can read a header a remote client forged.
func IngressHopMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hop := ingressHop{proto: r.Header.Get("X-Forwarded-Proto")}
		if vals := r.Header.Values(ingresstrust.HeaderName); len(vals) > 0 {
			hop.markerSeen = true
			hop.verified = len(vals) == 1 && ingresstrust.Valid(vals[0])
			r.Header.Del(ingresstrust.HeaderName)
		}
		ctx := context.WithValue(r.Context(), ingressHopKey{}, hop)
		if hop.verified {
			observeIngressPeer(r.RemoteAddr)
		}
		r = r.WithContext(ctx)
		if !peerIsTrustedProxy(r) {
			r.Header.Del("X-Forwarded-Proto")
			r.Header.Del("X-Forwarded-Host")
		}
		next.ServeHTTP(w, r)
	})
}

// ingressPeerObservation is the last TCP peer that delivered a verified
// ingress request, kept for the doctor check.
type ingressPeerObservation struct {
	addr string
	at   time.Time
}

var lastIngressPeer atomic.Pointer[ingressPeerObservation]

func observeIngressPeer(remoteAddr string) {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	lastIngressPeer.Store(&ingressPeerObservation{addr: host, at: time.Now()})
}

// insecureLoginReason explains why a request was not treated as HTTPS.
func insecureLoginReason(r *http.Request) string {
	hop := ingressHopOf(r)
	switch {
	case hop.verified:
		return "ingress hop verified but X-Forwarded-Proto is not https"
	case peerIsTrustedProxy(r):
		return "trusted peer but X-Forwarded-Proto is not https"
	case hop.markerSeen:
		return "ingress marker present but invalid, and peer is not loopback or in " + envTrustedProxies
	default:
		return "peer is not loopback, not in " + envTrustedProxies + " and carried no ingress marker (source address rewritten by NAT?)"
	}
}

var insecureLoginLog struct {
	mu   sync.Mutex
	last time.Time
}

// logInsecureLoginRefusal warns about a refused sign-in at most once per
// interval, so a scanner cannot flood the journal.
func (rt *Router) logInsecureLoginRefusal(r *http.Request) {
	interval := time.Minute
	if d, err := time.ParseDuration(strings.TrimSpace(os.Getenv(envInsecureLoginLogInterval))); err == nil && d >= 0 {
		interval = d
	}
	now := time.Now()
	insecureLoginLog.mu.Lock()
	if !insecureLoginLog.last.IsZero() && now.Sub(insecureLoginLog.last) < interval {
		insecureLoginLog.mu.Unlock()
		return
	}
	insecureLoginLog.last = now
	insecureLoginLog.mu.Unlock()
	rt.logger.Warn("api: refused sign-in over plain HTTP",
		slog.String("peer", r.RemoteAddr),
		slog.String("forwarded_proto", ingressHopOf(r).proto),
		slog.String("reason", insecureLoginReason(r)),
	)
}
