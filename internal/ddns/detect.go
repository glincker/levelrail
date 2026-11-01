package ddns

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

	"github.com/GLINCKER/levelrail/internal/heartbeat"
	"github.com/GLINCKER/levelrail/kit/netguard"
)

// Detection outcomes the loop reports as warnings, never as writes.
var (
	ErrNoQuorum  = errors.New("ddns: probes did not agree")
	ErrNonPublic = errors.New("ddns: detected address is not public")
)

// FetchFunc returns the raw answer of one probe URL.
type FetchFunc func(ctx context.Context, url string) (string, error)

// Detector asks several probe URLs and accepts an address only on agreement.
type Detector struct {
	URLs   []string
	Quorum int
	Fetch  FetchFunc
}

// NewDetector uses the guarded client, so a probe URL cannot point inside the network.
func NewDetector(cfg Config) *Detector {
	client := heartbeat.NewClient(cfg.AllowPrivate, cfg.Timeout)
	return &Detector{URLs: cfg.ProbeURLs, Quorum: cfg.Quorum, Fetch: clientFetch(client)}
}

func clientFetch(client *http.Client) FetchFunc {
	return func(ctx context.Context, u string) (string, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return "", errors.New("bad probe url")
		}
		resp, err := client.Do(req)
		if err != nil {
			return "", heartbeat.Scrub(err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("probe status %d", resp.StatusCode)
		}
		b, err := io.ReadAll(io.LimitReader(resp.Body, 128))
		return strings.TrimSpace(string(b)), err
	}
}

// Detect returns the IPv4 at least Quorum probes agree on.
func (d *Detector) Detect(ctx context.Context) (string, error) {
	var (
		mu     sync.Mutex
		counts = map[string]int{}
		wg     sync.WaitGroup
	)
	for _, u := range d.URLs {
		wg.Add(1)
		go func(u string) {
			defer wg.Done()
			raw, err := d.Fetch(ctx, u)
			if err != nil {
				return
			}
			addr, perr := netip.ParseAddr(raw)
			if perr != nil || !addr.Is4() {
				return
			}
			mu.Lock()
			counts[addr.String()]++
			mu.Unlock()
		}(u)
	}
	wg.Wait()
	need := max(d.Quorum, 1)
	for ip, n := range counts {
		if n >= need {
			if !isPublic(ip) {
				return "", ErrNonPublic
			}
			return ip, nil
		}
	}
	return "", ErrNoQuorum
}

var cgnat = netip.MustParsePrefix("100.64.0.0/10")

// isPublic refuses ranges a record must never point at, CGNAT included.
func isPublic(ip string) bool {
	addr, err := netip.ParseAddr(ip)
	if err != nil || netguard.IsBlocked(addr) || cgnat.Contains(addr) {
		return false
	}
	return addr.IsGlobalUnicast() && net.ParseIP(ip) != nil
}
