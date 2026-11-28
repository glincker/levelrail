package sources

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/memory"
	"golang.org/x/net/proxy"

	"github.com/glincker/levelrail/kit/netguard"
)

var errInflated = errors.New("sources: the repository inflates past the size cap")

// capStorage refuses any object, and any running total, that is larger once
// decoded than the caps. The wire budget only sees compressed bytes.
type capStorage struct {
	*memory.Storage
	maxObject int64
	maxTotal  int64
	maxCount  int64
	total     atomic.Int64
	count     atomic.Int64
}

func newCapStorage(l Limits) *capStorage {
	c := &capStorage{Storage: memory.NewStorage(), maxObject: l.MaxBytes, maxTotal: l.MaxBytes * 2}
	if l.MaxFiles > 0 {
		c.maxCount = int64(l.MaxFiles)*4 + 64
	}
	return c
}

func (c *capStorage) SetEncodedObject(obj plumbing.EncodedObject) (plumbing.Hash, error) {
	sz := obj.Size()
	if c.maxObject > 0 && (sz > c.maxObject || c.total.Add(sz) > c.maxTotal) {
		return plumbing.ZeroHash, errInflated
	}
	if c.maxCount > 0 && c.count.Add(1) > c.maxCount {
		return plumbing.ZeroHash, errInflated
	}
	return c.Storage.SetEncodedObject(obj)
}

const guardedSSHScheme = "lrguard"

var (
	sshOnce    sync.Once
	sshBudgets sync.Map
)

// installSSHDialer makes go-git ssh dial through the same connect-time address
// check as https, so a DNS answer that changed after CheckURL cannot reach
// an internal host. A proxy URL with a private scheme is the only dial hook.
func installSSHDialer() {
	sshOnce.Do(func() {
		c := netguard.NewClientAllowing()
		dial := c.Transport.(*http.Transport).DialContext
		proxy.RegisterDialerType(guardedSSHScheme, func(u *url.URL, _ proxy.Dialer) (proxy.Dialer, error) {
			b, _ := sshBudgets.Load(u.User.Username())
			left, _ := b.(*atomic.Int64)
			return guardDialer{dial: dial, left: left}, nil
		})
	})
}

type guardDialer struct {
	dial func(ctx context.Context, network, addr string) (net.Conn, error)
	left *atomic.Int64
}

func (g guardDialer) Dial(network, addr string) (net.Conn, error) {
	return g.DialContext(context.Background(), network, addr)
}

// DialContext hides why a dial failed: a refused port and a refused address look alike.
func (g guardDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	conn, err := g.dial(ctx, network, addr)
	if err != nil {
		return nil, errUnreachable
	}
	if g.left == nil {
		return conn, nil
	}
	return &budgetConn{Conn: conn, left: g.left}, nil
}

var errUnreachable = errors.New("sources: could not fetch the repository")

type budgetConn struct {
	net.Conn
	left *atomic.Int64
}

func (b *budgetConn) Read(p []byte) (int, error) {
	n, err := b.Conn.Read(p)
	if n > 0 && b.left.Add(-int64(n)) < 0 {
		return n, errBudget
	}
	return n, err
}

// sshProxyURL registers a per-fetch byte budget and returns the proxy URL that selects the guarded dialer.
func sshProxyURL(maxBytes int64) (string, func()) {
	var raw [8]byte
	_, _ = rand.Read(raw[:])
	token := hex.EncodeToString(raw[:])
	if maxBytes > 0 {
		left := &atomic.Int64{}
		left.Store(maxBytes)
		sshBudgets.Store(token, left)
	}
	return guardedSSHScheme + "://" + token + "@guard", func() { sshBudgets.Delete(token) }
}
