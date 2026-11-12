package secretconn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/secretsource"
)

// Policy narrows what a connection serves. Prefixes are path prefixes on whole
// segments; Envs are the names or ids an operator typed, and EnvIDs are what they resolved to
// at save. Only ids are compared, since names are not unique. Both empty means no restriction.
type Policy struct {
	AllowedPrefixes []string `json:"allowed_prefixes,omitempty"`
	AllowedEnvs     []string `json:"allowed_envs,omitempty"`
	AllowedEnvIDs   []string `json:"allowed_env_ids,omitempty"`
}

// IsZero reports whether no allow list is set.
func (p Policy) IsZero() bool {
	return len(p.AllowedPrefixes) == 0 && len(p.AllowedEnvs) == 0 && len(p.AllowedEnvIDs) == 0
}

func (p Policy) envRestricted() bool { return len(p.AllowedEnvs) > 0 || len(p.AllowedEnvIDs) > 0 }

// Delegable reports whether the lists are complete enough to let a non root writer use the connection.
func (p Policy) Delegable() bool { return len(p.AllowedPrefixes) > 0 && p.envRestricted() }

// EnvAllowed matches an environment id against the resolved ids; an unrestricted policy allows any.
func (p Policy) EnvAllowed(envID string) bool {
	if !p.envRestricted() {
		return true
	}
	if envID == "" {
		return false
	}
	for _, id := range p.AllowedEnvIDs {
		if id == envID {
			return true
		}
	}
	return false
}

// PathAllowed reports whether path equals a prefix or sits below it on a segment boundary,
// so "team-a" never admits "team-ab". An empty list allows any path.
func (p Policy) PathAllowed(path string) bool {
	if len(p.AllowedPrefixes) == 0 {
		return true
	}
	if !cleanPath(path) {
		return false
	}
	for _, pre := range p.AllowedPrefixes {
		if path == pre || strings.HasPrefix(path, pre+"/") {
			return true
		}
	}
	return false
}

// cleanPath refuses empty, dot and traversal segments, also after one round of percent decoding.
func cleanPath(path string) bool {
	if path == "" || strings.ContainsAny(path, "\\\x00#?") {
		return false
	}
	for _, form := range []string{path, unescaped(path)} {
		for _, seg := range strings.Split(form, "/") {
			if seg == "" || seg == "." || seg == ".." {
				return false
			}
		}
	}
	return true
}

func unescaped(s string) string {
	if u, err := url.PathUnescape(s); err == nil {
		return u
	}
	return s
}

// NormalizePolicy validates and canonicalizes the lists: trimmed, deduplicated, sorted,
// prefixes without edge slashes. Invalid entries are rejected rather than repaired.
func NormalizePolicy(p Policy) (Policy, error) {
	lim := secretsource.CurrentLimits()
	var out Policy
	seen := map[string]bool{}
	for _, raw := range p.AllowedPrefixes {
		pre := strings.Trim(strings.TrimSpace(raw), "/")
		if len(pre) > lim.MaxPath || !cleanPath(pre) {
			return Policy{}, invalid("allowed prefix %q must be a clean path without empty, dot or traversal segments", raw)
		}
		for _, seg := range strings.Split(pre, "/") {
			if err := (secretsource.Ref{Conn: "x", Path: seg, Key: "k"}).ValidateWith(lim); err != nil {
				return Policy{}, invalid("allowed prefix %q has a disallowed segment", raw)
			}
		}
		if !seen[pre] {
			seen[pre] = true
			out.AllowedPrefixes = append(out.AllowedPrefixes, pre)
		}
	}
	seen = map[string]bool{}
	for _, raw := range p.AllowedEnvs {
		env := strings.TrimSpace(raw)
		if env == "" || len(env) > lim.MaxConn || strings.ContainsAny(env, "\x00\n\r\t") {
			return Policy{}, invalid("allowed environment %q is empty, too long or has control characters", raw)
		}
		if k := strings.ToLower(env); !seen[k] {
			seen[k] = true
			out.AllowedEnvs = append(out.AllowedEnvs, env)
		}
	}
	seen = map[string]bool{}
	for _, raw := range p.AllowedEnvIDs {
		if id := strings.TrimSpace(raw); id != "" && !seen[id] {
			seen[id] = true
			out.AllowedEnvIDs = append(out.AllowedEnvIDs, id)
		}
	}
	sort.Strings(out.AllowedEnvIDs)
	sort.Strings(out.AllowedPrefixes)
	sort.Strings(out.AllowedEnvs)
	return out, nil
}

func parsePolicy(raw string) Policy {
	var p Policy
	_ = json.Unmarshal([]byte(raw), &p)
	return p
}

func policyJSON(p Policy) string {
	b, _ := json.Marshal(p)
	return string(b)
}

// resolveEnvIDs turns the operator's names or ids into environment ids. A name that matches
// several environments, or none, is refused so a look-alike environment can never inherit access.
func (s *Service) resolveEnvIDs(ctx context.Context, entries []string) ([]string, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	if s.Envs == nil {
		return nil, fmt.Errorf("%w: environment lookup unavailable", secretsource.ErrFailClosed)
	}
	var ids []string
	for _, e := range entries {
		if env, err := s.Envs.GetEnvironment(ctx, e); err == nil {
			ids = append(ids, env.ID)
			continue
		}
		found, err := s.Envs.EnvironmentIDsByName(ctx, e)
		if err != nil {
			return nil, fmt.Errorf("%w: environment lookup failed", secretsource.ErrFailClosed)
		}
		if len(found) != 1 {
			return nil, invalid("allowed environment %q matches %d environments; give its environment id", e, len(found))
		}
		ids = append(ids, found[0])
	}
	return ids, nil
}

// effectiveEnvIDs is the id list a check compares against. A policy saved before ids existed
// is resolved from its names now, and any ambiguity denies.
func (s *Service) effectiveEnvIDs(ctx context.Context, p Policy) (Policy, error) {
	if len(p.AllowedEnvIDs) > 0 || len(p.AllowedEnvs) == 0 {
		return p, nil
	}
	ids, err := s.resolveEnvIDs(ctx, p.AllowedEnvs)
	if err != nil {
		return p, err
	}
	p.AllowedEnvIDs = ids
	return p, nil
}

// DelegatedRefAllowed is the set-time check for a writer without root: the connection must
// carry both lists, the environment id must be listed and the path must sit inside a prefix.
func (s *Service) DelegatedRefAllowed(ctx context.Context, conn, envID, path string) bool {
	c, err := s.Get(ctx, conn)
	if err != nil || c.Legacy || c.Type == TypeDoppler {
		return false
	}
	pol, err := s.effectiveEnvIDs(ctx, c.Policy)
	if err != nil {
		return false
	}
	return pol.Delegable() && pol.EnvAllowed(envID) && pol.PathAllowed(path)
}

// CheckUnvettedRef gates a reference no actor vetted: only a delegating connection serves it.
func (s *Service) CheckUnvettedRef(ctx context.Context, conn, envID, path string) error {
	if !s.DelegatedRefAllowed(ctx, conn, envID, path) {
		return fmt.Errorf("%w: reference to connection %s was not set by root or inside its allow lists; set it again through the API", secretsource.ErrDenied, conn)
	}
	return nil
}

// CheckResolve is the resolve-time gate. A connection without a policy is unrestricted; one with
// a policy serves only listed environments and paths, and any lookup failure denies.
func (s *Service) CheckResolve(ctx context.Context, conn, envID, path string) error {
	c, err := s.Get(ctx, conn)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil // the resolver fails unknown connections closed itself
		}
		return fmt.Errorf("%w: connection %s policy unreadable", secretsource.ErrFailClosed, conn)
	}
	if c.Policy.IsZero() {
		return nil
	}
	pol, err := s.effectiveEnvIDs(ctx, c.Policy)
	if err != nil {
		return fmt.Errorf("%w: connection %s environment list cannot be resolved", secretsource.ErrFailClosed, conn)
	}
	if !pol.EnvAllowed(envID) {
		return fmt.Errorf("%w: connection %s does not allow this environment (see its allowed_envs)", secretsource.ErrDenied, conn)
	}
	if !pol.PathAllowed(path) {
		return fmt.Errorf("%w: connection %s does not allow this path (see its allowed_prefixes)", secretsource.ErrDenied, conn)
	}
	return nil
}

func (in Input) hasPolicy() bool { return in.AllowedPrefixes != nil || in.AllowedEnvs != nil }

// mergePolicy overlays the lists the input carries on base.
func (in Input) mergePolicy(base Policy) Policy {
	if in.AllowedPrefixes != nil {
		base.AllowedPrefixes = *in.AllowedPrefixes
	}
	if in.AllowedEnvs != nil {
		base.AllowedEnvs = *in.AllowedEnvs
		base.AllowedEnvIDs = nil // re-resolved at save
	}
	return base
}
