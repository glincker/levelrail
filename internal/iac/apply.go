package iac

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

// ErrPlanChanged means live state moved between the plan a user reviewed
// and the apply that followed.
var ErrPlanChanged = errors.New("the plan changed since it was reviewed; plan again")

// Item outcomes.
const (
	StatusApplied = "applied"
	StatusFailed  = "failed"
	StatusDenied  = "denied"
	StatusSkipped = "skipped"
	StatusNoop    = "noop"
)

// ItemResult is one plan item's outcome.
type ItemResult struct {
	Change
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// ApplyResult is the outcome of an apply.
type ApplyResult struct {
	Plan    Plan         `json:"plan"`
	Results []ItemResult `json:"results"`
	Applied int          `json:"applied"`
	Failed  int          `json:"failed"`
	Skipped int          `json:"skipped"`
}

// OK reports whether every item that had to change did.
func (r ApplyResult) OK() bool { return r.Failed == 0 && r.Skipped == 0 }

type executor struct {
	d          Doer
	opts       Options
	st         *State
	projectIDs map[string]string
	envIDs     map[string]string
	appsDone   map[string]bool
	appsFailed map[string]bool
}

func (x *executor) do(ctx context.Context, method, path string, body, out any) error {
	return x.d.Do(ctx, method, path, body, out)
}

func (x *executor) projectID(name string) (string, error) {
	if id, ok := x.projectIDs[name]; ok {
		return id, nil
	}
	if p, ok := x.st.projects[name]; ok {
		return p.ID, nil
	}
	return "", fmt.Errorf("project %q was not found", name)
}

func (x *executor) environmentID(project, name string) (string, error) {
	if id, ok := x.envIDs[envKey(project, name)]; ok {
		return id, nil
	}
	if e, ok := x.st.envs[envKey(project, name)]; ok {
		return e.ID, nil
	}
	return "", fmt.Errorf("environment %q of project %q was not found", name, project)
}

func (x *executor) putEnv(ctx context.Context, path string, live, desired map[string]string) error {
	if len(desired) == 0 {
		return nil
	}
	return x.do(ctx, http.MethodPut, path, mergeEnv(live, desired), nil)
}

func (x *executor) ensureTag(ctx context.Context, name string) (string, error) {
	if t, ok := x.st.tags[name]; ok {
		return t.ID, nil
	}
	var out wireTag
	if err := x.do(ctx, http.MethodPost, "/api/v1/tags", map[string]string{"name": name}, &out); err != nil {
		var se *StatusError
		if errors.As(err, &se) && se.Status == http.StatusConflict {
			return "", nil
		}
		return "", err
	}
	x.st.tags[name] = out
	return out.ID, nil
}

func (x *executor) setDatabaseProject(ctx context.Context, db, project string) error {
	if project == "" {
		return nil
	}
	pid, err := x.projectID(project)
	if err != nil {
		return err
	}
	return x.do(ctx, http.MethodPut, "/api/v1/databases/"+esc(db)+"/project", map[string]string{"project_id": pid}, nil)
}

func (x *executor) addDomain(ctx context.Context, app, host string) error {
	var raw map[string]any
	if err := x.do(ctx, http.MethodGet, "/api/v1/apps/"+esc(app), nil, &raw); err != nil {
		return err
	}
	domains, _ := raw["domains"].([]any)
	for _, d := range domains {
		if d == host {
			return nil
		}
	}
	raw["domains"] = append(domains, host)
	return x.do(ctx, http.MethodPut, "/api/v1/apps/"+esc(app), raw, nil)
}

// Apply plans and executes resources through doer. Validation and reading
// happen before any write; a plan holding an error item applies nothing
// unless ContinueOnError is set. Items run in dependency order and each
// goes through the ordinary endpoint, so a denied write is reported on its
// own item and the rest proceed only when ContinueOnError is set.
func Apply(ctx context.Context, d Doer, res []*Resource, opts Options, expectedHash string) (ApplyResult, error) {
	st, err := Load(ctx, d, res, opts)
	if err != nil {
		return ApplyResult{}, err
	}
	plan, items := buildPlan(st, res, opts)
	if expectedHash != "" && expectedHash != plan.Hash {
		return ApplyResult{Plan: plan}, ErrPlanChanged
	}
	out := ApplyResult{Plan: plan}
	if plan.Summary.Error > 0 && !opts.ContinueOnError {
		for _, it := range items {
			s := StatusSkipped
			switch it.Action {
			case ActionError:
				s = StatusFailed
				out.Failed++
			case ActionNoop:
				s = StatusNoop
			default:
				out.Skipped++
			}
			out.Results = append(out.Results, ItemResult{Change: it.Change, Status: s, Error: it.Reason})
		}
		return out, nil
	}
	x := &executor{d: d, opts: opts, st: st, projectIDs: map[string]string{}, envIDs: map[string]string{}, appsDone: map[string]bool{}, appsFailed: map[string]bool{}}
	stopped := false
	for _, it := range items {
		res := runItem(ctx, x, it, stopped)
		switch res.Status {
		case StatusApplied:
			out.Applied++
		case StatusFailed, StatusDenied:
			out.Failed++
			stopped = stopped || !opts.ContinueOnError
		case StatusSkipped:
			out.Skipped++
		}
		out.Results = append(out.Results, res)
	}
	return out, nil
}

func runItem(ctx context.Context, x *executor, it *item, stopped bool) ItemResult {
	res := ItemResult{Change: it.Change}
	switch {
	case it.Action == ActionNoop:
		res.Status = StatusNoop
	case it.Action == ActionError:
		res.Status = StatusFailed
		res.Error = it.Reason
	case stopped:
		res.Status = StatusSkipped
		res.Error = "not attempted after an earlier failure"
	case it.exec == nil:
		res.Status = StatusNoop
	default:
		if err := it.exec(ctx, x); err != nil {
			res.Status, res.Error = StatusFailed, err.Error()
			if IsDenied(err) {
				res.Status = StatusDenied
			}
			if it.Kind == KindApp {
				x.appsFailed[it.Name] = true
			}
		} else {
			res.Status = StatusApplied
			if it.Kind == KindApp {
				x.appsDone[it.Name] = true
			}
		}
	}
	return res
}
