package network

// This file: the DNS wire protocol half of TASKS.md 3.4's internal DNS.
// The decisions live in dns.go; this only speaks the protocol.
//
// Authoritative-only, deliberately. This server answers for its own zone
// and nothing else: no recursion, no forwarding, no upstream. A container
// therefore needs this resolver *in addition to* whatever it already
// uses for public names, not instead of it, which is exactly how
// Docker's own --dns list works (resolvers are tried in order). The
// alternative, forwarding everything not in our zone, would make this
// process a full recursive resolver for every container on the node: a
// much larger surface, a new failure mode for every public DNS lookup an
// app makes, and no benefit, since the node already has a working
// resolver configured.
//
// A container can now be pointed at this server: docker.ContainerSpec.DNS,
// the agent's protobuf ContainerSpec.dns, and internal/docker.Client.Create
// all carry it through, and every reconcile controller
// (application/database/nginxdemo) accepts a WithMeshDNSAddr option that
// sets it. cmd/levelrail/mesh.go's containerDNSAddr is what actually
// resolves a real address for that option, and it is deliberately
// conservative about when it will: Docker's DNS HostConfig field and every
// container's own resolv.conf only ever accept a bare nameserver IP, never
// "ip:port" (confirmed against a real daemon: an "ip:port" entry passes
// ContainerCreate uninspected but fails ContainerStart outright), and there
// is no way to ask a container's resolver to query a non-standard port
// either. So a running mesh only actually gets wired into containers when
// its DNS server is bound to :53 (APP_MESH_DNS_ADDR), not the unprivileged
// default port this package's own tests and a bare `go run` use. Until an
// operator sets that, this server stays reachable and correct but unused by
// containers, exactly as before this wiring existed; it can still be
// queried directly (dig @<mesh-ip> web.<zone>) to verify the mesh is
// resolving, which is what its own test does.

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/miekg/dns"
)

// dnsRecordTTL is how long a resolver may cache an answer from this
// server.
//
// Short on purpose. A record's answer changes exactly when a service is
// moved between nodes, which is a rare event, but it is also the event
// this entire subsystem exists to survive, and a cached stale answer
// means the move looks like an outage for the length of the TTL. Five
// seconds keeps the window under a single reconcile interval; the cost is
// query volume, which for a handful of internal names is nothing.
const dnsRecordTTL = 5 * time.Second

// DNSServer serves A records for a Resolver's zone over UDP and TCP.
type DNSServer struct {
	resolver *Resolver
	logger   *slog.Logger

	mu      sync.Mutex
	udp     *dns.Server
	tcp     *dns.Server
	udpAddr netip.AddrPort
	started bool

	// closed is atomic rather than guarded by mu because the serve
	// goroutines read it on their way out while Start may still be
	// holding mu waiting for those same goroutines to signal readiness.
	// Guarding it with mu would deadlock exactly when a listener fails to
	// activate, which is the one case the readiness wait exists for.
	closed atomic.Bool
}

// NewDNSServer builds a server for resolver. It does not listen until
// Start is called.
func NewDNSServer(resolver *Resolver, logger *slog.Logger) *DNSServer {
	if logger == nil {
		logger = slog.Default()
	}
	return &DNSServer{resolver: resolver, logger: logger}
}

// Start binds addr and begins serving on both UDP and TCP.
//
// Both, not just UDP: a response larger than 512 bytes forces a client to
// retry over TCP, and a UDP-only server turns that into a hang rather
// than a retry. Answers here are small enough today that it should never
// happen, which is exactly why it would be an unpleasant surprise later.
//
// addr of the form "host:0" picks a free port, which is what the tests
// use; Addr reports the port actually bound.
func (s *DNSServer) Start(ctx context.Context, addr string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed.Load() {
		return fmt.Errorf("network: start dns server: %w", ErrMeshClosed)
	}
	if s.started {
		return fmt.Errorf("network: start dns server: already started on %s", s.udpAddr)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("network: start dns server on %s: %w", addr, err)
	}

	mux := dns.NewServeMux()
	mux.HandleFunc(".", s.handle)

	udpConn, tcpListener, err := s.bindPair(addr)
	if err != nil {
		return err
	}
	bound := udpConn.LocalAddr().String()

	if ap, perr := netip.ParseAddrPort(bound); perr == nil {
		s.udpAddr = ap
	}

	// Both servers signal when they are actually in their serve loop, and
	// Start waits for both before returning. Without this, a Close that
	// follows Start closely enough races the serve goroutines and gets
	// "server not started" back from Shutdown, which is a real failure
	// mode (a control plane that starts and immediately shuts down on a
	// startup error), not just a test artifact.
	udpReady := make(chan struct{})
	tcpReady := make(chan struct{})
	// Buffered for both goroutines so neither blocks on send if Start has
	// already returned and nobody is receiving.
	failed := make(chan error, 2)
	s.udp = &dns.Server{PacketConn: udpConn, Handler: mux, NotifyStartedFunc: func() { close(udpReady) }}
	s.tcp = &dns.Server{Listener: tcpListener, Handler: mux, NotifyStartedFunc: func() { close(tcpReady) }}
	s.started = true

	go s.serve(s.udp, "udp", failed)
	go s.serve(s.tcp, "tcp", failed)

	for _, w := range []struct {
		proto string
		ready <-chan struct{}
	}{{"udp", udpReady}, {"tcp", tcpReady}} {
		select {
		case <-w.ready:
		case err := <-failed:
			// A listener that never activates must not leave Start
			// blocked forever waiting for a readiness signal that is
			// never coming.
			return fmt.Errorf("network: dns %s listener on %s failed to start: %w", w.proto, bound, err)
		case <-ctx.Done():
			return fmt.Errorf("network: waiting for the dns %s listener on %s: %w", w.proto, bound, ctx.Err())
		}
	}

	s.logger.Info("internal dns listening",
		slog.String("addr", bound), slog.String("zone", s.resolver.Zone()))
	return nil
}

// bindPair binds the same host and port on both UDP and TCP.
//
// The retry is not defensive padding. UDP and TCP have separate port
// spaces, so when addr asks for port 0 the kernel can hand back a UDP
// port that is already taken on TCP, and the second bind fails with
// "address already in use" through no fault of the caller. Binding one
// protocol first and the other second has this problem whichever order
// it is done in; the only fix is to let go of both and ask again. With
// an explicit port there is nothing to retry, so a failure there is
// reported immediately rather than being tried five times.
func (s *DNSServer) bindPair(addr string) (net.PacketConn, net.Listener, error) {
	const attempts = 5

	ephemeral := strings.HasSuffix(addr, ":0")
	var lastErr error

	for i := 0; i < attempts; i++ {
		udpConn, err := net.ListenPacket("udp", addr)
		if err != nil {
			return nil, nil, fmt.Errorf("network: listen udp for dns on %s: %w", addr, err)
		}
		bound := udpConn.LocalAddr().String()

		tcpListener, err := net.Listen("tcp", bound)
		if err == nil {
			return udpConn, tcpListener, nil
		}
		lastErr = err
		if cerr := udpConn.Close(); cerr != nil {
			s.logger.Error("closing dns udp listener after a failed tcp bind",
				slog.String("addr", bound), slog.String("error", cerr.Error()))
		}
		if !ephemeral {
			break
		}
	}
	return nil, nil, fmt.Errorf("network: listen tcp for dns on %s: %w", addr, lastErr)
}

func (s *DNSServer) serve(srv *dns.Server, proto string, failed chan<- error) {
	err := srv.ActivateAndServe()
	if err == nil {
		return
	}
	// Shutdown closes the listener out from under ActivateAndServe, so an
	// error here after Close is expected, not a failure.
	if s.closed.Load() {
		return
	}
	s.logger.Error("internal dns server stopped",
		slog.String("proto", proto), slog.String("error", err.Error()))
	select {
	case failed <- err:
	default:
	}
}

// Addr reports the address the server actually bound, which is how a
// caller that asked for port 0 finds out what it got.
func (s *DNSServer) Addr() netip.AddrPort {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.udpAddr
}

// Close shuts both listeners down. Idempotent.
func (s *DNSServer) Close() error {
	if s.closed.Swap(true) {
		return nil
	}
	s.mu.Lock()
	udp, tcp := s.udp, s.tcp
	s.mu.Unlock()

	var firstErr error
	for _, srv := range []*dns.Server{udp, tcp} {
		if srv == nil {
			continue
		}
		if err := srv.Shutdown(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("network: shut down dns server: %w", err)
		}
	}
	return firstErr
}

// handle answers one query.
func (s *DNSServer) handle(w dns.ResponseWriter, req *dns.Msg) {
	resp := new(dns.Msg)
	resp.SetReply(req)
	resp.Authoritative = true
	resp.RecursionAvailable = false

	if len(req.Question) != 1 {
		// A multi-question query is legal to send and universally
		// unsupported to answer; every real resolver sends exactly one.
		resp.Rcode = dns.RcodeFormatError
		s.write(w, resp)
		return
	}
	q := req.Question[0]

	if !s.resolver.Authoritative(q.Name) {
		// Not our zone at all. REFUSED rather than NXDOMAIN so the
		// client knows to ask someone else, instead of concluding the
		// name does not exist anywhere. See this file's header.
		resp.Rcode = dns.RcodeRefused
		s.write(w, resp)
		return
	}

	switch q.Qtype {
	case dns.TypeA:
		addr, ok := s.resolver.Lookup(q.Name)
		if !ok || !addr.Is4() {
			resp.Rcode = dns.RcodeNameError
			break
		}
		resp.Answer = append(resp.Answer, &dns.A{
			Hdr: dns.RR_Header{
				Name:   q.Name,
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    uint32(dnsRecordTTL.Seconds()),
			},
			A: net.IP(addr.AsSlice()),
		})

	case dns.TypeAAAA:
		// The mesh is IPv4 only (see validateCIDR). An empty NOERROR,
		// not NXDOMAIN: the name exists, it just has no AAAA. Returning
		// NXDOMAIN here would make a dual-stack client that happens to
		// ask AAAA first conclude the name does not exist and never ask
		// for the A record, which is a real and confusing failure.
		if _, ok := s.resolver.Lookup(q.Name); !ok {
			resp.Rcode = dns.RcodeNameError
		}

	default:
		// Any other type for a name in our zone: NOERROR with no answer,
		// same reasoning as AAAA.
		if _, ok := s.resolver.Lookup(q.Name); !ok {
			resp.Rcode = dns.RcodeNameError
		}
	}

	s.write(w, resp)
}

func (s *DNSServer) write(w dns.ResponseWriter, resp *dns.Msg) {
	if err := w.WriteMsg(resp); err != nil {
		s.logger.Warn("writing dns response",
			slog.String("error", err.Error()), slog.String("zone", s.resolver.Zone()))
	}
}
