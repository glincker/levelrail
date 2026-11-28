package monitor

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"os"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"

	"github.com/glincker/levelrail/kit/netguard"
)

// checkDNS passes when the name resolves to at least one address. Latency is the lookup time.
func (c *Checker) checkDNS(ctx context.Context, m Monitor, res *Result) {
	addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip", m.Spec.Host)
	if err != nil {
		res.Reason = classify(err)
		if res.Reason == ReasonConnect {
			res.Reason = ReasonNoAnswer
		}
		return
	}
	if len(addrs) == 0 {
		res.Reason = ReasonNoAnswer
		return
	}
	res.OK = true
}

// checkPing sends one ICMP echo over IPv4. It tries the unprivileged datagram socket first so the
// control plane needs no raw socket right, then a raw socket.
func (c *Checker) checkPing(ctx context.Context, m Monitor, res *Result) {
	dst, reason := c.pingTarget(ctx, m.Spec.Host)
	if reason != "" {
		res.Reason = reason
		return
	}
	network, addr := "udp4", net.Addr(&net.UDPAddr{IP: dst.AsSlice()})
	conn, err := icmp.ListenPacket(network, "0.0.0.0")
	if err != nil {
		network, addr = "ip4:icmp", &net.IPAddr{IP: dst.AsSlice()}
		if conn, err = icmp.ListenPacket(network, "0.0.0.0"); err != nil {
			res.Reason = ReasonConnect
			return
		}
	}
	defer func() { _ = conn.Close() }()
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = c.now().Add(DefaultMaxTimeout)
	}
	_ = conn.SetDeadline(deadline)
	id := os.Getpid() & 0xffff
	req, err := (&icmp.Message{Type: ipv4.ICMPTypeEcho, Body: &icmp.Echo{ID: id, Seq: 1, Data: []byte("levelrail")}}).Marshal(nil)
	if err != nil {
		res.Reason = ReasonBadRequest
		return
	}
	if _, err := conn.WriteTo(req, addr); err != nil {
		res.Reason = classify(err)
		return
	}
	buf := make([]byte, 1500)
	for {
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			res.Reason = ReasonNoReply
			if ctx.Err() != nil || isTimeout(err) {
				res.Reason = ReasonTimeout
			}
			return
		}
		reply, err := icmp.ParseMessage(1, buf[:n])
		if err == nil && reply.Type == ipv4.ICMPTypeEchoReply {
			res.OK = true
			return
		}
	}
}

// pingTarget resolves host to one IPv4 address the guard allows, or returns a failure reason.
func (c *Checker) pingTarget(ctx context.Context, host string) (netip.Addr, string) {
	addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip4", host)
	if err != nil || len(addrs) == 0 {
		return netip.Addr{}, ReasonNoAnswer
	}
	for _, a := range addrs {
		if netguard.AllowPrivate() || !netguard.IsBlocked(a) {
			return a, ""
		}
	}
	return netip.Addr{}, ReasonBlocked
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}
