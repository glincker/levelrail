package sourcecache

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/client"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
)

// subScheme routes submodule fetches through the guarded client below; the
// shared https transport other code uses is left alone.
const subScheme = "lrsub"

var (
	errAddressBlocked = errors.New("address not allowed")
	errBudgetExceeded = errors.New("size limit reached")
	cgnat             = netip.MustParsePrefix("100.64.0.0/10")
	thisNet           = netip.MustParsePrefix("0.0.0.0/8")
)

// netPolicy travels in the request context: internal addresses are refused
// unless allowed, and response bodies draw from a shared byte budget.
type netPolicy struct {
	allowed []netip.Prefix
	budget  *atomic.Int64
}

type policyKey struct{}

func withPolicy(ctx context.Context, p *netPolicy) context.Context {
	return context.WithValue(ctx, policyKey{}, p)
}

func policyOf(ctx context.Context) *netPolicy {
	p, _ := ctx.Value(policyKey{}).(*netPolicy)
	return p
}

func parseCIDRs(items []string) []netip.Prefix {
	var out []netip.Prefix
	for _, it := range items {
		it = strings.TrimSpace(it)
		if p, err := netip.ParsePrefix(it); err == nil {
			out = append(out, p.Masked())
		} else if a, err := netip.ParseAddr(it); err == nil {
			out = append(out, netip.PrefixFrom(a.Unmap(), a.Unmap().BitLen()))
		}
	}
	return out
}

// addressBlocked reports whether ip is internal and not explicitly allowed.
func addressBlocked(ip netip.Addr, allowed []netip.Prefix) bool {
	ip = ip.Unmap()
	for _, p := range allowed {
		if p.Contains(ip) {
			return false
		}
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() || cgnat.Contains(ip) || thisNet.Contains(ip)
}

// dialControl checks the address actually being connected to, after DNS,
// so a rebinding answer cannot reach an internal host.
func dialControl(allowed []netip.Prefix) func(network, address string, _ syscall.RawConn) error {
	return func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return errAddressBlocked
		}
		ip, err := netip.ParseAddr(host)
		if err != nil || addressBlocked(ip, allowed) {
			return errAddressBlocked
		}
		return nil
	}
}

// guardedTransport dials only public addresses (plus the policy allowlist)
// and refuses any request that carries no policy.
func guardedTransport() http.RoundTripper {
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.Proxy = nil
	base.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		p := policyOf(ctx)
		if p == nil {
			return nil, errAddressBlocked
		}
		d := &net.Dialer{Timeout: 30 * time.Second, Control: dialControl(p.allowed)}
		return d.DialContext(ctx, network, addr)
	}
	return countingTransport{base}
}

type countingTransport struct{ inner http.RoundTripper }

func (t countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.inner.RoundTrip(req)
	if err != nil || resp.Body == nil {
		return resp, err
	}
	if p := policyOf(req.Context()); p != nil && p.budget != nil {
		resp.Body = &countingBody{ReadCloser: resp.Body, budget: p.budget}
	}
	return resp, nil
}

// countingBody fails a read once the shared budget is spent, so an oversized
// pack or error body stops mid-transfer instead of after it lands on disk.
type countingBody struct {
	io.ReadCloser
	budget *atomic.Int64
}

func (b *countingBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 && b.budget.Add(-int64(n)) < 0 {
		return n, errBudgetExceeded
	}
	return n, err
}

// subTransport serves the lrsub scheme as https over the guarded client.
type subTransport struct{ inner transport.Transport }

func (t subTransport) NewUploadPackSession(ep *transport.Endpoint, auth transport.AuthMethod) (transport.UploadPackSession, error) {
	e := *ep
	e.Protocol = "https"
	if e.Port == 0 {
		e.Port = 443
	}
	return t.inner.NewUploadPackSession(&e, auth)
}

func (subTransport) NewReceivePackSession(*transport.Endpoint, transport.AuthMethod) (transport.ReceivePackSession, error) {
	return nil, errors.New("sourcecache: submodules are read only")
}

var installSub sync.Once

func installSubProtocol() {
	installSub.Do(func() {
		client.InstallProtocol(subScheme, subTransport{githttp.NewClient(&http.Client{Transport: guardedTransport(), CheckRedirect: noRedirect})})
	})
}

func noRedirect(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

var (
	guardedLFSOnce sync.Once
	guardedLFS     *http.Client
)

func defaultLFSClient() *http.Client {
	guardedLFSOnce.Do(func() { guardedLFS = &http.Client{Transport: guardedTransport()} })
	return guardedLFS
}

// unreachable is the one message tenants see for any network failure; the
// cause (refused, timeout, blocked address) stays in the server log.
func (c *Cache) unreachable(kind, host string, err error) error {
	c.log.Warn("sourcecache: "+kind+" fetch failed", "host", host, "error", oneLine(err.Error()))
	return fmt.Errorf("%w: %s", ErrSubmoduleUnreachable, host)
}
