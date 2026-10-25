package iac

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/envredact"
)

// jobOp is one job write an app item performs after the app itself.
type jobOp struct {
	name   string
	method string
	id     string
	want   jobFields
}

func desiredJobs(f map[string]any) map[string]jobFields {
	raw, ok := f["jobs"]
	if !ok {
		return nil
	}
	out := map[string]jobFields{}
	if err := remarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

func desiredConns(f map[string]any) map[string]string {
	out := map[string]string{}
	if m, ok := f["connections"].(map[string]any); ok {
		for k, v := range m {
			if s, ok := v.(string); ok {
				out[k] = s
			}
		}
	}
	return out
}

// checkConnRefs fails the item when a referenced database is neither live
// nor declared, and returns the databases the app must wait for.
func (p *planner) checkConnRefs(r *Resource, f map[string]any) ([]string, bool) {
	conns := desiredConns(f)
	var deps []string
	for _, env := range sortedKeys(conns) {
		db, _ := splitConnRef(conns[env])
		_, live := p.st.databases[db]
		_, declared := p.byKey[resourceKey(KindDatabase, "", db)]
		_, unreadable := p.st.denied["databases"]
		if !live && !declared && !unreadable {
			p.fail(r, "env %s reads from database %q, which does not exist and is not declared in the files", env, db)
			return nil, false
		}
		deps = append(deps, resourceKey(KindDatabase, "", db))
	}
	return sortedUnique(deps), true
}

func connWarnings(conns map[string]string, live *liveApp) []string {
	if live == nil {
		return nil
	}
	var out []string
	for _, env := range sortedKeys(conns) {
		_, literal := live.wire.Env[env]
		if literal || live.secrets[env] {
			out = append(out, fmt.Sprintf("env %s is also set directly on the live app; remove that value so only the database connection supplies it", env))
		}
	}
	return out
}

// planJobs diffs declared jobs by name. Jobs absent from the files are
// kept, or removed under prune; jobs owned by app.yaml are never touched.
func (p *planner) planJobs(r *Resource, want map[string]jobFields, live *liveApp, prune bool) (changes, kept []FieldChange, ops []jobOp) {
	if want == nil {
		return nil, nil, nil
	}
	have := map[string]wireJob{}
	if live != nil {
		have = live.jobs
	}
	for _, name := range sortedKeys(want) {
		w := want[name]
		cur, exists := have[name]
		if exists && cur.Source == jobSourceSpec {
			r.Warnings = append(r.Warnings, fmt.Sprintf("job %s is declared in the app's app.yaml and is left as it is; declare it in one place", name))
			continue
		}
		var oldMap map[string]any
		if exists {
			oldMap = toMap(jobFromWire(cur))
		}
		c, _ := diffFields(map[string]any{"jobs." + name: toMap(w)}, map[string]any{"jobs." + name: oldMap}, nil, false)
		if len(c) == 0 {
			continue
		}
		changes = append(changes, c...)
		op := jobOp{name: name, method: http.MethodPost, want: w}
		if exists {
			op.method, op.id = http.MethodPut, cur.ID
		}
		ops = append(ops, op)
	}
	for _, name := range sortedKeys(have) {
		if _, declared := want[name]; declared || have[name].Source == jobSourceSpec {
			continue
		}
		fc := FieldChange{Path: "jobs." + name, Op: OpKeep, Old: have[name].Schedule}
		if prune {
			fc.Op = OpRemove
			changes = append(changes, fc)
			ops = append(ops, jobOp{name: name, method: http.MethodDelete, id: have[name].ID})
			continue
		}
		kept = append(kept, fc)
	}
	for i := range changes {
		if strings.HasSuffix(changes[i].Path, ".command") && envredact.SensitiveValue("COMMAND", changes[i].Old) {
			changes[i].Old = hiddenValue
		}
	}
	sort.SliceStable(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return changes, kept, ops
}

func (x *executor) syncJobs(ctx context.Context, app string, ops []jobOp) error {
	base := "/api/v1/apps/" + esc(app) + "/jobs"
	for _, op := range ops {
		var err error
		switch op.method {
		case http.MethodPost:
			err = x.do(ctx, http.MethodPost, base, jobBody(op.name, op.want), nil)
		case http.MethodPut:
			err = x.do(ctx, http.MethodPut, base+"/"+esc(op.id), jobBody(op.name, op.want), nil)
		case http.MethodDelete:
			err = x.do(ctx, http.MethodDelete, base+"/"+esc(op.id), nil, nil)
		}
		if err != nil {
			return fmt.Errorf("job %s: %w", op.name, err)
		}
	}
	return nil
}

// syncConnections writes database references through the connections API.
// Only names travel; the control plane resolves values at container create.
func (x *executor) syncConnections(ctx context.Context, app string, want map[string]string, live *wireApp, prune bool) (bool, error) {
	base := "/api/v1/apps/" + esc(app) + "/connections"
	have := map[string]string{}
	if live != nil {
		for k, ref := range live.DatabaseEnv {
			have[k] = ref.Database + "." + ref.Field
		}
	}
	changed := false
	for _, env := range sortedKeys(want) {
		if have[env] == want[env] {
			continue
		}
		db, field := splitConnRef(want[env])
		if err := x.do(ctx, http.MethodPost, base, map[string]string{"database": db, "field": field, "env_var": env}, nil); err != nil {
			return changed, fmt.Errorf("connect %s to database %s: %w", env, db, err)
		}
		changed = true
	}
	if !prune {
		return changed, nil
	}
	for _, env := range sortedKeys(have) {
		if _, ok := want[env]; ok {
			continue
		}
		if err := x.do(ctx, http.MethodDelete, base+"/"+esc(env), nil, nil); err != nil {
			return changed, fmt.Errorf("disconnect %s: %w", env, err)
		}
		changed = true
	}
	return changed, nil
}
