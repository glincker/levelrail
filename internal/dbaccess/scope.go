package dbaccess

import (
	"errors"
	"fmt"
)

// Scope is how far a database's network reach extends.
type Scope string

// Scopes. Platform is the historical behaviour: any app that references the
// database is attached to it.
const (
	ScopePlatform    Scope = "platform"
	ScopeProject     Scope = "project"
	ScopeEnvironment Scope = "environment"
)

// ErrScopeUnplaced is returned when the database has no project or
// environment to scope to.
var ErrScopeUnplaced = errors.New("database has nothing to scope to")

// Valid reports whether s is a known scope.
func (s Scope) Valid() bool {
	return s == ScopePlatform || s == ScopeProject || s == ScopeEnvironment
}

// Placement is where a database or an app sits in the platform's grouping.
type Placement struct {
	Name          string `json:"name"`
	ProjectID     string `json:"project_id,omitempty"`
	EnvironmentID string `json:"environment_id,omitempty"`
	// ProjectName and EnvironmentName are only used to word reasons.
	ProjectName     string `json:"project_name,omitempty"`
	EnvironmentName string `json:"environment_name,omitempty"`
}

// Verdict is one referencing app's fate under a scope.
type Verdict struct {
	App     Placement `json:"app"`
	Allowed bool      `json:"allowed"`
	Reason  string    `json:"reason,omitempty"`
}

// Allows reports whether app may reach a database placed at db under scope.
func Allows(scope Scope, db, app Placement) (bool, string) {
	switch scope {
	case ScopeProject:
		if app.ProjectID == db.ProjectID {
			return true, ""
		}
		return false, fmt.Sprintf("app %q is in project %s, the database is scoped to project %s", app.Name,
			orNone(first(app.ProjectName, app.ProjectID)), orNone(first(db.ProjectName, db.ProjectID)))
	case ScopeEnvironment:
		if app.EnvironmentID == db.EnvironmentID {
			return true, ""
		}
		return false, fmt.Sprintf("app %q is in environment %s, the database is scoped to environment %s", app.Name,
			orNone(first(app.EnvironmentName, app.EnvironmentID)), orNone(first(db.EnvironmentName, db.EnvironmentID)))
	default:
		return true, ""
	}
}

func first(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func orNone(v string) string {
	if v == "" {
		return "none"
	}
	return v
}

// DryRun evaluates every referencing app under scope without changing
// anything. It errors when the database has no placement to scope to.
func DryRun(scope Scope, db Placement, apps []Placement) ([]Verdict, error) {
	if !scope.Valid() {
		return nil, fmt.Errorf("unknown scope %q", scope)
	}
	switch {
	case scope == ScopeProject && db.ProjectID == "":
		return nil, fmt.Errorf("%w: assign the database to a project first", ErrScopeUnplaced)
	case scope == ScopeEnvironment && db.EnvironmentID == "":
		return nil, fmt.Errorf("%w: assign the database to an environment first", ErrScopeUnplaced)
	}
	out := make([]Verdict, 0, len(apps))
	for _, a := range apps {
		ok, why := Allows(scope, db, a)
		out = append(out, Verdict{App: a, Allowed: ok, Reason: why})
	}
	return out, nil
}

// Lost returns the verdicts that would lose reachability.
func Lost(vs []Verdict) []Verdict {
	var out []Verdict
	for _, v := range vs {
		if !v.Allowed {
			out = append(out, v)
		}
	}
	return out
}
