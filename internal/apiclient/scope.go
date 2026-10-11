package apiclient

import (
	"bufio"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
)

// Scope headers narrow what the control plane shows to one team or project.
// They can only narrow: the server never widens access from them.
const (
	ScopeTeamHeader    = "X-Scope-Team"
	ScopeProjectHeader = "X-Scope-Project"
	EnvTeam            = "APP_TEAM"
	EnvProject         = "APP_PROJECT"
	contextFileName    = "context"
)

// Scope is the team and project every request is narrowed to.
type Scope struct {
	Team    string
	Project string
}

var activeScope atomic.Value

// SetScope sets the scope every Client sends from now on.
func SetScope(s Scope) { activeScope.Store(s) }

// CurrentScope returns the scope set by SetScope.
func CurrentScope() Scope {
	s, _ := activeScope.Load().(Scope)
	return s
}

type scopeTransport struct{ base http.RoundTripper }

func (t scopeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	s := CurrentScope()
	if s.Team == "" && s.Project == "" {
		return t.base.RoundTrip(req)
	}
	clone := req.Clone(req.Context())
	if s.Team != "" {
		clone.Header.Set(ScopeTeamHeader, s.Team)
	}
	if s.Project != "" {
		clone.Header.Set(ScopeProjectHeader, s.Project)
	}
	return t.base.RoundTrip(clone)
}

// ResolveScope picks team and project by precedence: flag, env, then the
// profile's saved context.
func ResolveScope(flags Scope, lookupEnv func(string) (string, bool), prog, profile string) Scope {
	saved, _ := ReadContext(prog, profile)
	pick := func(flag, env, stored string) string {
		if flag != "" {
			return flag
		}
		if v, ok := lookupEnv(env); ok && v != "" {
			return v
		}
		return stored
	}
	return Scope{Team: pick(flags.Team, EnvTeam, saved.Team), Project: pick(flags.Project, EnvProject, saved.Project)}
}

func contextPath(prog string) (string, error) {
	dir, err := ConfigDir(prog)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, contextFileName), nil
}

// ReadContext returns a profile's saved team and project.
func ReadContext(prog, profile string) (Scope, error) {
	all, err := readContexts(prog)
	if err != nil {
		return Scope{}, err
	}
	return all[profile], nil
}

// WriteContext saves a profile's team and project; an empty scope clears it.
func WriteContext(prog, profile string, s Scope) error {
	all, err := readContexts(prog)
	if err != nil {
		return err
	}
	if s.Team == "" && s.Project == "" {
		delete(all, profile)
	} else {
		all[profile] = s
	}
	path, err := contextPath(prog)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("apiclient: create config dir: %w", err)
	}
	var b strings.Builder
	for name, sc := range all {
		fmt.Fprintf(&b, "%s team=%s project=%s\n", name, sc.Team, sc.Project)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return fmt.Errorf("apiclient: write context: %w", err)
	}
	return nil
}

func readContexts(prog string) (map[string]Scope, error) {
	out := map[string]Scope{}
	path, err := contextPath(prog)
	if err != nil {
		return out, err
	}
	f, err := os.Open(path) //nolint:gosec // path is under the user's own config dir
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return out, fmt.Errorf("apiclient: read context: %w", err)
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 0 {
			continue
		}
		var s Scope
		for _, kv := range fields[1:] {
			k, v, _ := strings.Cut(kv, "=")
			switch k {
			case "team":
				s.Team = v
			case "project":
				s.Project = v
			}
		}
		out[fields[0]] = s
	}
	return out, sc.Err()
}
