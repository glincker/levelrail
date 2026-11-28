// Package sources syncs private template sources: a git repo whose templates
// are validated like built-ins and namespaced so they never replace one.
package sources

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/glincker/levelrail/internal/gitsshkey"
	"github.com/glincker/levelrail/kit/netguard"
)

// LookupFunc resolves a host to its addresses; tests replace it.
type LookupFunc func(ctx context.Context, host string) ([]netip.Addr, error)

// DefaultLookup resolves through the system resolver with a short timeout.
func DefaultLookup(ctx context.Context, host string) ([]netip.Addr, error) {
	c, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return net.DefaultResolver.LookupNetIP(c, "ip", host)
}

// Errors returned by CheckURL.
var (
	ErrURLScheme   = errors.New("sources: only https and ssh git URLs are allowed")
	ErrURLCreds    = errors.New("sources: the URL may not carry a password or token, use a deploy key")
	ErrURLInternal = errors.New("sources: the host is not a public address")
	ErrURLInvalid  = errors.New("sources: not a valid git URL")
)

var scpLike = regexp.MustCompile(`^([A-Za-z0-9._-]+)@([A-Za-z0-9.-]+):([^/].*)$`)

// CheckURL accepts https and ssh git URLs whose host resolves only to public
// addresses. The fetch dial re-checks at connect time, so a later DNS change
// cannot move a source onto an internal address.
func CheckURL(ctx context.Context, raw string, lookup LookupFunc) (ssh bool, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, " \t\r\n\x00") {
		return false, ErrURLInvalid
	}
	var host string
	if m := scpLike.FindStringSubmatch(raw); m != nil && !strings.Contains(raw, "://") {
		host, ssh = m[2], true
	} else {
		u, perr := url.Parse(raw)
		if perr != nil || u.Host == "" {
			return false, ErrURLInvalid
		}
		switch u.Scheme {
		case "https":
			if u.User != nil {
				return false, ErrURLCreds
			}
		case "ssh":
			if _, hasPw := u.User.Password(); hasPw {
				return false, ErrURLCreds
			}
			ssh = true
		default:
			return false, ErrURLScheme
		}
		host = u.Hostname()
	}
	if ssh && !gitsshkey.IsSSHURL(raw) {
		return false, ErrURLInvalid
	}
	if err := checkHost(ctx, host, lookup); err != nil {
		return false, err
	}
	return ssh, nil
}

func checkHost(ctx context.Context, host string, lookup LookupFunc) error {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "" || host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".internal") {
		return fmt.Errorf("%w: %s", ErrURLInternal, host)
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		if netguard.IsBlocked(ip) {
			return fmt.Errorf("%w: %s", ErrURLInternal, host)
		}
		return nil
	}
	if lookup == nil {
		lookup = DefaultLookup
	}
	addrs, err := lookup(ctx, host)
	if err != nil || len(addrs) == 0 {
		return fmt.Errorf("%w: %s does not resolve", ErrURLInvalid, host)
	}
	for _, a := range addrs {
		if netguard.IsBlocked(a) {
			return fmt.Errorf("%w: %s resolves to a private, loopback or link-local address", ErrURLInternal, host)
		}
	}
	return nil
}
