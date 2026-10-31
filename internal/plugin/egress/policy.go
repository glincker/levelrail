package egress

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync/atomic"
)

// PluginPolicy is one plugin's credential hash (hex SHA-256) and allowed hosts.
// Hosts are "host" (port 443) or "host:port", as StaticAllowlist expects.
type PluginPolicy struct {
	Name       string   `json:"name"`
	SecretHash string   `json:"secret_hash"`
	Hosts      []string `json:"hosts"`
}

// Policy is the full egress policy the control plane pushes to the sidecar.
type Policy struct {
	Version int64          `json:"version"`
	Plugins []PluginPolicy `json:"plugins"`
}

// Validate rejects any policy the sidecar must not apply.
func (p Policy) Validate() error {
	if p.Version < 0 {
		return errors.New("policy version must not be negative")
	}
	seen := make(map[string]bool, len(p.Plugins))
	for _, pl := range p.Plugins {
		if pl.Name == "" {
			return errors.New("plugin name is empty")
		}
		if strings.Contains(pl.Name, ":") {
			return fmt.Errorf("plugin name %q must not contain a colon", pl.Name)
		}
		if seen[pl.Name] {
			return fmt.Errorf("duplicate plugin %q", pl.Name)
		}
		seen[pl.Name] = true
		if _, err := parseSecretHash(pl.SecretHash); err != nil {
			return fmt.Errorf("plugin %q: %w", pl.Name, err)
		}
		for _, h := range pl.Hosts {
			if err := validateHostEntry(h); err != nil {
				return fmt.Errorf("plugin %q: %w", pl.Name, err)
			}
		}
	}
	return nil
}

func parseSecretHash(s string) (SecretHash, error) {
	var out SecretHash
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != len(out) {
		return out, errors.New("secret hash must be 64 hex characters")
	}
	copy(out[:], b)
	return out, nil
}

// HexHash is the wire form of a SecretHash.
func HexHash(h SecretHash) string { return hex.EncodeToString(h[:]) }

func validateHostEntry(e string) error {
	host := e
	if i := strings.LastIndexByte(e, ':'); i >= 0 {
		host = e[:i]
		n, err := strconv.Atoi(e[i+1:])
		if err != nil || n < 1 || n > 65535 {
			return fmt.Errorf("invalid port in host %q", e)
		}
	}
	if host == "" || strings.ContainsAny(host, " \t\r\n/\\@*[]:?#") {
		return fmt.Errorf("invalid host %q", e)
	}
	return nil
}

// EncodePolicy validates and serializes a policy.
func EncodePolicy(p Policy) ([]byte, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(p)
}

// DecodePolicy parses strictly (unknown fields and trailing data are errors)
// and validates.
func DecodePolicy(r io.Reader) (Policy, error) {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	var p Policy
	if err := dec.Decode(&p); err != nil {
		return Policy{}, fmt.Errorf("decode policy: %w", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return Policy{}, errors.New("decode policy: trailing data")
	}
	if err := p.Validate(); err != nil {
		return Policy{}, err
	}
	return p, nil
}

type compiledPolicy struct {
	version int64
	creds   *HashedCredentials
	allow   StaticAllowlist
}

// PolicyStore holds the active policy and swaps it atomically. With no policy
// it denies everything. It implements CredentialStore and Allowlist.
type PolicyStore struct {
	cur atomic.Pointer[compiledPolicy]
}

// NewPolicyStore returns a store with no policy (deny all).
func NewPolicyStore() *PolicyStore { return &PolicyStore{} }

// Set validates and installs a policy in one atomic swap.
func (s *PolicyStore) Set(p Policy) error {
	if err := p.Validate(); err != nil {
		return err
	}
	hashes := make(map[string]SecretHash, len(p.Plugins))
	allow := make(StaticAllowlist, len(p.Plugins))
	for _, pl := range p.Plugins {
		h, err := parseSecretHash(pl.SecretHash)
		if err != nil {
			return err
		}
		hashes[pl.Name] = h
		allow[pl.Name] = append([]string(nil), pl.Hosts...)
	}
	s.cur.Store(&compiledPolicy{version: p.Version, creds: NewHashedCredentials(hashes), allow: allow})
	return nil
}

// Ready reports whether a policy has been installed.
func (s *PolicyStore) Ready() bool { return s.cur.Load() != nil }

// Version returns the installed policy version, or -1 with none.
func (s *PolicyStore) Version() int64 {
	if c := s.cur.Load(); c != nil {
		return c.version
	}
	return -1
}

// Verify implements CredentialStore.
func (s *PolicyStore) Verify(plugin, secret string) bool {
	c := s.cur.Load()
	if c == nil {
		return false
	}
	return c.creds.Verify(plugin, secret)
}

// Allowed implements Allowlist.
func (s *PolicyStore) Allowed(plugin, host string, port int) bool {
	c := s.cur.Load()
	if c == nil {
		return false
	}
	return c.allow.Allowed(plugin, host, port)
}

func decodeBytes(b []byte) (Policy, error) { return DecodePolicy(bytes.NewReader(b)) }
