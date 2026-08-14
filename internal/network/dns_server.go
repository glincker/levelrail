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
// Known, deliberate gap: nothing points a container at this server yet.
// Doing that means adding a DNS field to docker.ContainerSpec, to the
// agent's protobuf ContainerSpec, and to the Engine API call in
// internal/docker.Client.Create. That change crosses the agent wire
// contract, which CLAUDE.md section 8 puts on the do-not-parallelize
// list, and it is small and mechanical enough to be worth landing as its
// own reviewed change rather than buried in this one. Until then this
// server is reachable and correct but unused by containers; it can be
// queried directly (dig @<mesh-ip> web.<zone>) to verify the mesh is
// resolving, which is what its test does.

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"sync"
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
	closed  bool
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
	if s.closed {
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

	udpConn, err := net.ListenPacket("udp", addr)
	if err != nil {
		return fmt.Errorf("network: listen udp for dns on %s: %w", addr, err)
	}
	bound := udpConn.LocalAddr().String()
	tcpListener, err := net.Listen("tcp", bound)
	if err != nil {
		if cerr := udpConn.Close(); cerr != nil {
			s.logger.Error("closing dns udp listener after failed tcp bind",
				slog.String("addr", bound), slog.String("error", cerr.Error()))
		}
		return fmt.Errorf("network: listen tcp for dns on %s: %w", bound, err)
	}

	if ap, perr := netip.ParseAddrPort(bound); perr == nil {
		s.udpAddr = ap
	}
	s.udp = &dns.Server{PacketConn: udpConn, Handler: mux}
	s.tcp = &dns.Server{Listener: tcpListener, Handler: mux}
	s.started = true

	go s.serve(s.udp, "udp")
	go s.serve(s.tcp, "tcp")

	s.logger.Info("internal dns listening",
		slog.String("addr", bound), slog.String("zone", s.resolver.Zone()))
	return nil
}

func (s *DNSServer) serve(srv *dns.Server, proto string) {
	if err := srv.ActivateAndServe(); err != nil {
		// Shutdown closes the listener out from under ActivateAndServe,
		// so an error here after Close is expected, not a failure. The
		// closed flag distinguishes the two.
		s.mu.Lock()
		closed := s.closed
		s.mu.Unlock()
		if !closed {
			s.logger.Error("internal dns server stopped",
				slog.String("proto", proto), slog.String("error", err.Error()))
		}
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
	s.mu.Lock()
	udp, tcp := s.udp, s.tcp
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
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
