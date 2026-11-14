package iac

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/GLINCKER/levelrail/internal/compose"
)

// ComposeBody is sent as the raw compose file instead of JSON. Env feeds
// ${VAR} interpolation on the server and never reaches the stored revision.
type ComposeBody struct {
	Data []byte
	Env  map[string]string
}

// ComposeServiceRow is one display only preview row of a compose App.
type ComposeServiceRow struct {
	Service string `json:"service"`
	Action  Action `json:"action"`
	Image   string `json:"image,omitempty"`
}

// liveCompose is the latest stored revision of a compose App. The server
// stores the body as received, so its digest compares with the declared one.
type liveCompose struct {
	body     []byte
	profiles []string
	services []string
}

func (l loader) composeOf(ctx context.Context, name string) error {
	key := "compose/" + name
	var list struct {
		Revisions []struct {
			Revision int `json:"revision"`
		} `json:"revisions"`
	}
	if err := l.get(ctx, "/api/v1/apps/"+esc(name)+"/compose/revisions", &list); err != nil {
		if IsNotFound(err) {
			return nil
		}
		return l.st.note(key, fmt.Errorf("read compose history of app %s: %w", name, err))
	}
	if len(list.Revisions) == 0 {
		return nil
	}
	var rev struct {
		Compose  string   `json:"compose"`
		Profiles []string `json:"profiles"`
	}
	if err := l.get(ctx, fmt.Sprintf("/api/v1/apps/%s/compose/revisions/%d", esc(name), list.Revisions[0].Revision), &rev); err != nil {
		return l.st.note(key, fmt.Errorf("read compose revision of app %s: %w", name, err))
	}
	lc := &liveCompose{body: []byte(rev.Compose), profiles: sortedUnique(rev.Profiles)}
	if f, err := compose.Parse(lc.body); err == nil {
		if _, err := f.ApplyProfiles(lc.profiles); err == nil {
			lc.services = sortedKeys(f.Services)
		}
	}
	l.st.compose[name] = lc
	return l.protection(ctx, name)
}

func (p *planner) planComposeApp(r *Resource) {
	in := r.compose
	if p.opts.NoDeploy {
		p.failCode(r, CodeComposeNoDeployUnsupported, "compose App %s deploys on every save, so no_deploy cannot be honored; apply without it", r.Name)
		return
	}
	if len(r.SecretRefs) > 0 || in.indirect {
		p.failCode(r, CodeUnavailable, "spec.compose.env of app %s uses a secretRef or a secret reference; only literal values apply in this release", r.Name)
		return
	}
	if !p.checkAppRefs(r, copyFields(r.Fields)) {
		return
	}
	if err, denied := p.st.denied["compose/"+r.Name]; denied {
		p.failCode(r, CodeReadFailed, "the compose history of app %s cannot be read: %v", r.Name, err)
		return
	}
	exec := func(ctx context.Context, x *executor) error { return x.deployCompose(ctx, r) }
	live := p.st.compose[r.Name]
	if live == nil {
		it := p.add(r, ActionCreate, []FieldChange{{Path: "compose.digest", Op: OpAdd, New: shortDigest(r.ComposeDigest)}}, nil, exec)
		it.Services = composeRows(in.images, nil)
		return
	}
	if p.st.protected[r.Name] {
		p.failCode(r, CodeProtectedResource, "app %s is protected; a compose redeploy can remove services, so unprotect it first", r.Name)
		return
	}
	have := composeDigest(live.body, live.profiles, in.names)
	envNow := composeEnvDigest(in.env)
	applied, known := p.opts.ComposeEnvApplied[r.Name]
	// Without a record of the last applied env (a plain apply), declared env redeploys every time.
	envSame := envNow == "" || (known && applied == envNow)
	if have == r.ComposeDigest && envSame {
		p.add(r, ActionNoop, nil, nil, nil)
		return
	}
	var changes []FieldChange
	if have != r.ComposeDigest {
		changes = append(changes, FieldChange{Path: "compose.digest", Op: OpChange, Old: shortDigest(have), New: shortDigest(r.ComposeDigest)})
	}
	if !envSame {
		old := "unknown"
		if known && applied != "" {
			old = shortEnvDigest(applied)
		}
		changes = append(changes, FieldChange{Path: "compose.env", Op: OpChange, Old: old, New: shortEnvDigest(envNow)})
	}
	it := p.add(r, ActionUpdate, changes, nil, exec)
	it.Services = composeRows(in.images, live.services)
}

// composeEnvDigest keys the sorted NAME=value pairs with the install hash key,
// so a low entropy value cannot be guessed back from the digest.
func composeEnvDigest(env map[string]string) string {
	if len(env) == 0 {
		return ""
	}
	mac := hmac.New(sha256.New, currentHashKey())
	for _, k := range sortedKeys(env) {
		mac.Write([]byte(k + "=" + env[k] + "\x00"))
	}
	return hex.EncodeToString(mac.Sum(nil))
}

func shortEnvDigest(d string) string {
	if len(d) > 12 {
		return "hmac:" + d[:12]
	}
	return d
}

// ComposeEnvDigests returns the env digest of every compose App the files
// declare, so a stack can record what its last applied revision set.
func ComposeEnvDigests(files []Source, project string) map[string]string {
	docs, att, issues := SplitComposeRefs(files)
	if len(issues) > 0 {
		return nil
	}
	parsed, issues := ParseDocuments(docs)
	if len(issues) > 0 {
		return nil
	}
	if project != "" {
		parsed = FilterProject(parsed, project)
	}
	res, issues := Build(parsed, Options{Attachments: att})
	if len(issues) > 0 {
		return nil
	}
	out := map[string]string{}
	for _, r := range res {
		if r.compose != nil {
			out[r.Name] = composeEnvDigest(r.compose.env)
		}
	}
	return out
}

// planComposePrune deletes a compose App the stack owns and the files dropped.
// It needs the typed confirmation, and the teardown keeps named volumes.
func (p *planner) planComposePrune(r *Resource, live *liveCompose) *item {
	name, stack := r.Name, p.st.ledger.StackName
	if !p.opts.ConfirmDestroy {
		it := p.add(r, ActionError, nil, nil, nil)
		it.Code = CodeDestroyNotConfirmed
		it.Reason = fmt.Sprintf("compose app %s is owned by stack %s and absent from the files; deleting it removes %d services, so send confirm_destroy with the stack name", name, stack, len(live.services))
		it.Services = composeRows(nil, live.services)
		return nil
	}
	it := p.add(r, ActionDelete, nil, nil, func(ctx context.Context, x *executor) error {
		if !x.opts.ConfirmDestroy {
			return errors.New(CodeDestroyNotConfirmed + ": a compose app delete needs confirm_destroy typed with the stack name")
		}
		// No volumes=true: named volumes outlive the app unless removed on purpose.
		return x.do(ctx, http.MethodDelete, "/api/v1/apps/"+esc(name)+"/compose", nil, nil)
	})
	it.Reason = "compose app owned by stack " + stack + " and absent from the files; every service is removed, named volumes are kept"
	it.Services = composeRows(nil, live.services)
	return it
}

func shortDigest(d string) string {
	if len(d) > 12 {
		return "sha256:" + d[:12]
	}
	return d
}

// composeRows lists create and update for declared services and remove for
// live ones the file dropped, since a compose redeploy prunes those.
func composeRows(want map[string]string, live []string) []ComposeServiceRow {
	var rows []ComposeServiceRow
	for _, name := range sortedKeys(want) {
		act := ActionCreate
		if slices.Contains(live, name) {
			act = ActionUpdate
		}
		rows = append(rows, ComposeServiceRow{Service: name, Action: act, Image: want[name]})
	}
	for _, name := range live {
		if _, ok := want[name]; !ok {
			rows = append(rows, ComposeServiceRow{Service: name, Action: ActionDelete})
		}
	}
	return rows
}

// deployCompose sends the attachment through the compose deploy endpoint,
// refusing when the bytes no longer match the digest the plan approved.
func (x *executor) deployCompose(ctx context.Context, r *Resource) error {
	in := r.compose
	data, ok := x.opts.Attachments[in.path]
	if !ok || composeDigest(data, in.profiles, in.names) != r.ComposeDigest {
		return fmt.Errorf("%s: compose file %s changed after the plan; plan again", CodePlanStale, in.path)
	}
	app := "/api/v1/apps/" + esc(r.Name)
	target := app + "/compose"
	if len(in.profiles) > 0 {
		target += "?profiles=" + url.QueryEscape(strings.Join(in.profiles, ","))
	}
	if err := x.do(ctx, http.MethodPost, target, ComposeBody{Data: data, Env: maps.Clone(in.env)}, nil); err != nil {
		return err
	}
	if proj := stringField(r.Fields, "project"); proj != "" {
		pid, err := x.projectID(proj)
		if err != nil {
			return err
		}
		if err := x.do(ctx, http.MethodPut, app+"/project", map[string]string{"project_id": pid}, nil); err != nil {
			return err
		}
		if env := stringField(r.Fields, "environment"); env != "" {
			eid, err := x.environmentID(proj, env)
			if err != nil {
				return err
			}
			if err := x.do(ctx, http.MethodPut, app+"/environment", map[string]string{"environment_id": eid}, nil); err != nil {
				return err
			}
		}
	}
	var live *wireApp
	if la, ok := x.st.apps[r.Name]; ok {
		live = &la.wire
	}
	return x.syncTags(ctx, r.Name, stringList(r.Fields["tags"]), live, false)
}
