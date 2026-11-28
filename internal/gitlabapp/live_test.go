package gitlabapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/gitprovider"
)

// Live tests hit the real gitlab.com. They run only when LEVELRAIL_GITLAB_LIVE_TOKEN
// is set (never committed). LEVELRAIL_GITLAB_LIVE_URL overrides the instance, and
// LEVELRAIL_GITLAB_LIVE_GROUP_ID reuses an existing group instead of creating one.

const liveHookURL = "https://example.invalid/levelrail-live-hook"

type liveEnv struct {
	t        *testing.T
	c        *Client
	ctx      context.Context
	inst     string
	tok      string
	groupID  int64
	groupURL string
}

func (e *liveEnv) redact(err error) string {
	if err == nil {
		return ""
	}
	return strings.ReplaceAll(err.Error(), e.tok, "[redacted]")
}

// call is setup plumbing for endpoints the client has no function for.
func (e *liveEnv) call(method, path string, body any, out any) error {
	var rd *bytes.Reader
	var err error
	if body != nil {
		b, merr := json.Marshal(body)
		if merr != nil {
			return merr
		}
		rd = bytes.NewReader(b)
	}
	u := apiBaseURL(e.inst) + path
	if rd == nil {
		err = e.c.do(e.ctx, method, u, "Bearer "+e.tok, nil, out)
	} else {
		err = e.c.do(e.ctx, method, u, "Bearer "+e.tok, rd, out)
	}
	return err
}

func (e *liveEnv) step(name string, fn func() error) bool {
	ok := true
	e.t.Run(name, func(t *testing.T) {
		if err := fn(); err != nil {
			ok = false
			t.Errorf("FAIL %s: %s", name, e.redact(err))
			return
		}
		t.Logf("PASS %s", name)
	})
	return ok
}

func TestLiveGitLab(t *testing.T) {
	tok := os.Getenv("LEVELRAIL_GITLAB_LIVE_TOKEN")
	if tok == "" {
		t.Skip("LEVELRAIL_GITLAB_LIVE_TOKEN not set")
	}
	inst := os.Getenv("LEVELRAIL_GITLAB_LIVE_URL")
	if inst == "" {
		inst = "https://gitlab.com"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	e := &liveEnv{t: t, c: NewClient(), ctx: ctx, inst: inst, tok: tok}
	suffix := strconv.FormatInt(time.Now().UnixNano()%1e9, 36)

	e.step("token_info_scopes", func() error {
		ti, err := e.c.TokenInfo(ctx, inst, tok)
		if err == nil {
			t.Logf("scopes=%v", ti.Scopes)
		}
		return err
	})
	e.step("current_user", func() error {
		a, err := e.c.CurrentUser(ctx, inst, tok)
		if err == nil {
			t.Logf("login=%s", a.Login)
		}
		return err
	})

	// Everything lives in one throwaway group, deleted at the end even on failure.
	if gid := os.Getenv("LEVELRAIL_GITLAB_LIVE_GROUP_ID"); gid != "" {
		e.groupID, _ = strconv.ParseInt(gid, 10, 64)
	} else {
		var g struct {
			ID      int64  `json:"id"`
			WebURL  string `json:"web_url"`
			Message string `json:"message"`
		}
		name := "lr-qa-" + suffix
		if !e.step("create_group", func() error {
			return e.call(http.MethodPost, "/groups", map[string]string{"name": name, "path": name, "visibility": "private"}, &g)
		}) {
			t.Fatalf("cannot create the throwaway group, aborting before any project is created")
		}
		e.groupID = g.ID
		defer func() {
			if err := e.call(http.MethodDelete, "/groups/"+strconv.FormatInt(g.ID, 10), nil, nil); err != nil {
				t.Errorf("group cleanup failed: %s", e.redact(err))
			}
		}()
	}

	e.step("list_groups", func() error {
		var gs []struct {
			ID       int64  `json:"id"`
			FullPath string `json:"full_path"`
		}
		if err := e.call(http.MethodGet, "/groups?min_access_level=10", nil, &gs); err != nil {
			return err
		}
		for _, g := range gs {
			if g.ID == e.groupID {
				return nil
			}
		}
		return fmt.Errorf("created group %d missing from %d listed groups", e.groupID, len(gs))
	})

	var proj struct {
		ID                int64  `json:"id"`
		PathWithNamespace string `json:"path_with_namespace"`
		DefaultBranch     string `json:"default_branch"`
	}
	if !e.step("create_project", func() error {
		return e.call(http.MethodPost, "/projects", map[string]any{
			"name": "app-" + suffix, "namespace_id": e.groupID, "visibility": "private",
			"initialize_with_readme": true, "container_registry_access_level": "enabled",
		}, &proj)
	}) {
		return
	}
	pid, path := proj.ID, proj.PathWithNamespace
	defer func() { _ = e.call(http.MethodDelete, "/projects/"+strconv.FormatInt(pid, 10), nil, nil) }()

	e.step("list_projects_dashboard_fn", func() error {
		ps, _, err := e.c.ListProjects(ctx, inst, tok, gitprovider.ListOptions{})
		if err != nil {
			return err
		}
		for _, p := range ps {
			if p.ID == pid {
				return nil
			}
		}
		return fmt.Errorf("new project %d not in ListProjects result (%d items)", pid, len(ps))
	})
	e.step("get_project", func() error {
		p, err := e.c.GetProject(ctx, inst, tok, pid)
		if err == nil && p.PathWithNamespace != path {
			return fmt.Errorf("path %q != %q", p.PathWithNamespace, path)
		}
		return err
	})

	var branch string
	e.step("list_branches", func() error {
		bs, err := e.c.ListBranches(ctx, inst, tok, pid)
		if err != nil {
			return err
		}
		if len(bs) == 0 {
			return fmt.Errorf("no branches")
		}
		branch = bs[0].Name
		return nil
	})
	if branch == "" {
		branch = "main"
	}

	var sha string
	e.step("push_commit_via_api", func() error {
		var out struct {
			ID string `json:"id"`
		}
		err := e.call(http.MethodPost, "/projects/"+strconv.FormatInt(pid, 10)+"/repository/commits", map[string]any{
			"branch": "feature", "start_branch": branch, "commit_message": "live test commit",
			"actions": []map[string]string{{"action": "create", "file_path": "hello.txt", "content": "hi\n"}},
		}, &out)
		sha = out.ID
		return err
	})

	e.step("webhook_defaults_push_only", func() error {
		var h struct {
			ID                    int64 `json:"id"`
			PushEvents            bool  `json:"push_events"`
			TagPushEvents         bool  `json:"tag_push_events"`
			MergeRequestsEvents   bool  `json:"merge_requests_events"`
			EnableSSLVerification bool  `json:"enable_ssl_verification"`
		}
		base := "/projects/" + strconv.FormatInt(pid, 10) + "/hooks"
		if err := e.call(http.MethodPost, base, map[string]any{"url": liveHookURL + "-defaults", "push_events": true}, &h); err != nil {
			return err
		}
		t.Logf("defaults: tag_push=%v merge_requests=%v ssl_verify=%v", h.TagPushEvents, h.MergeRequestsEvents, h.EnableSSLVerification)
		return e.call(http.MethodDelete, base+"/"+strconv.FormatInt(h.ID, 10), nil, nil)
	})

	e.step("create_webhook_client_fn_and_read_back", func() error {
		if err := e.c.CreateProjectWebhook(ctx, inst, tok, pid, liveHookURL, "live-secret"); err != nil {
			return err
		}
		var hs []struct {
			URL                   string `json:"url"`
			PushEvents            bool   `json:"push_events"`
			TagPushEvents         bool   `json:"tag_push_events"`
			MergeRequestsEvents   bool   `json:"merge_requests_events"`
			EnableSSLVerification bool   `json:"enable_ssl_verification"`
		}
		if err := e.call(http.MethodGet, "/projects/"+strconv.FormatInt(pid, 10)+"/hooks", nil, &hs); err != nil {
			return err
		}
		for _, h := range hs {
			if h.URL == liveHookURL {
				t.Logf("client hook: push=%v tag_push=%v mr=%v ssl=%v", h.PushEvents, h.TagPushEvents, h.MergeRequestsEvents, h.EnableSSLVerification)
				if !h.PushEvents || !h.MergeRequestsEvents || !h.EnableSSLVerification {
					return fmt.Errorf("hook missing requested events: %+v", h)
				}
				if !h.TagPushEvents {
					return fmt.Errorf("tag_push_events is off on the auto-registered hook (release trigger mode will not fire)")
				}
				return nil
			}
		}
		return fmt.Errorf("created hook not found among %d hooks", len(hs))
	})

	e.step("commit_status", func() error {
		if sha == "" {
			return fmt.Errorf("no sha from the push step")
		}
		if err := e.c.CreateCommitStatus(ctx, inst, tok, path, sha, CommitStateSuccess, "https://example.invalid/p", "live", "levelrail/preview"); err != nil {
			return err
		}
		sts, err := e.c.ListCommitStatuses(ctx, inst, tok, path, sha, "feature")
		if err == nil && len(sts) == 0 {
			return fmt.Errorf("status posted but ListCommitStatuses returned none")
		}
		return err
	})

	var iid int
	e.step("create_merge_request", func() error {
		var mr struct {
			IID int `json:"iid"`
		}
		err := e.call(http.MethodPost, "/projects/"+strconv.FormatInt(pid, 10)+"/merge_requests", map[string]any{
			"source_branch": "feature", "target_branch": branch, "title": "live test mr",
		}, &mr)
		iid = mr.IID
		return err
	})
	e.step("mr_changed_files", func() error {
		fs, err := e.c.MergeRequestChangedFiles(ctx, inst, tok, path, iid)
		if err == nil && len(fs) == 0 {
			return fmt.Errorf("no changed files reported for the MR")
		}
		return err
	})
	e.step("mr_note_upsert", func() error {
		id, err := e.c.CreateMergeRequestNote(ctx, inst, tok, path, iid, "preview: https://example.invalid")
		if err != nil {
			return err
		}
		notes, err := e.c.ListMergeRequestNotes(ctx, inst, tok, path, iid)
		if err != nil {
			return err
		}
		if len(notes) == 0 {
			return fmt.Errorf("note created but not listed")
		}
		return e.c.UpdateMergeRequestNote(ctx, inst, tok, path, iid, id, "preview: updated")
	})

	e.step("registry_listing", func() error {
		var repos []map[string]any
		return e.call(http.MethodGet, "/projects/"+strconv.FormatInt(pid, 10)+"/registry/repositories", nil, &repos)
	})
}
