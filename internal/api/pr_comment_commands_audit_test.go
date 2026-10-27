package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func (f *prCmdFixture) refusals() []prCommandRefusalDetail {
	f.t.Helper()
	entries, err := f.db.ListAuditEntries(context.Background(), 200, nil, store.AuditEntryFilter{})
	if err != nil {
		f.t.Fatalf("ListAuditEntries() error = %v", err)
	}
	var out []prCommandRefusalDetail
	for _, e := range entries {
		if e.Action != auditActionPRCommandRefused {
			continue
		}
		if strings.Contains(e.Detail, "SECRETMARKER") || strings.Contains(e.Path, "SECRETMARKER") {
			f.t.Errorf("audit row stored comment body text: %+v", e)
		}
		var d prCommandRefusalDetail
		if err := json.Unmarshal([]byte(e.Detail), &d); err != nil {
			f.t.Fatalf("audit detail %q: %v", e.Detail, err)
		}
		if e.ActorName != prCommentActorTmpl+d.Login {
			f.t.Errorf("actor name = %q, want the commenter login", e.ActorName)
		}
		out = append(out, d)
	}
	return out
}

func (f *prCmdFixture) wantOneRefusal(reason string) prCommandRefusalDetail {
	f.t.Helper()
	got := f.refusals()
	if len(got) != 1 {
		f.t.Fatalf("refusal rows = %+v, want exactly one with reason %q", got, reason)
	}
	d := got[0]
	if d.Reason != reason || d.App != "web" || d.Repo != "org/web" || d.PR != 7 || d.Login == "" {
		f.t.Fatalf("refusal = %+v, want reason %q for web, org/web, pr 7 and a login", d, reason)
	}
	return d
}

const prCmdAuditBody = "/levelrail redeploy\nSECRETMARKER do not store me"

func TestPRCommentAudit_Refusals(t *testing.T) {
	disabled := testPRCommentConfig()
	disabled.Disabled = true
	tests := []struct {
		name   string
		cfg    PRCommentCommandConfig
		on     bool
		setup  func(*prCmdFixture)
		o      commentOpts
		reason string
	}{
		{"kill switch", disabled, true, nil, commentOpts{}, prRefusedCommandsOff},
		{"app setting off", testPRCommentConfig(), false, nil, commentOpts{}, prRefusedCommandsOff},
		{"no write access", testPRCommentConfig(), true, func(f *prCmdFixture) { f.gh.perms["octo"] = "read" }, commentOpts{}, prRefusedNoWriteAccess},
		{"not a collaborator answers 404", testPRCommentConfig(), true, func(f *prCmdFixture) { delete(f.gh.perms, "octo") }, commentOpts{}, prRefusedPermissionLookup},
		{"permission lookup failed", testPRCommentConfig(), true, func(f *prCmdFixture) { f.gh.permStatus = http.StatusInternalServerError }, commentOpts{}, prRefusedPermissionLookup},
		{"bot", testPRCommentConfig(), true, nil, commentOpts{login: "evil<b>[bot]"}, prRefusedBot},
		{"fork", testPRCommentConfig(), true, func(f *prCmdFixture) {
			f.gh.pr["head"].(map[string]any)["repo"] = map[string]any{"full_name": "fork/web"}
		}, commentOpts{}, prRefusedFork},
		{"frozen", testPRCommentConfig(), true, func(f *prCmdFixture) {
			if _, err := f.db.ReplaceDeployFreezeWindows(context.Background(), store.DeployFreezeScopeApp("web"),
				[]store.DeployFreezeWindow{{Cron: "* * * * *", Duration: 2 * time.Hour, Reason: "quarter end <b>x</b>"}}); err != nil {
				t.Fatal(err)
			}
		}, commentOpts{}, prRefusedBlocked},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newPRCmdFixture(t, tt.cfg, tt.on)
			if tt.setup != nil {
				tt.setup(f)
			}
			tt.o.body = prCmdAuditBody
			f.send(tt.o)
			if len(f.builder.calls) != 0 {
				t.Fatal("a refused command deployed")
			}
			d := f.wantOneRefusal(tt.reason)
			if d.Command != "redeploy" {
				t.Errorf("command = %q, want redeploy", d.Command)
			}
			for _, bad := range []string{"<", ">", "@", "["} {
				if strings.Contains(d.Login+d.Note, bad) {
					t.Errorf("audit echoes %q unsanitized: %+v", bad, d)
				}
			}
			if tt.reason == prRefusedBlocked && !strings.Contains(d.Note, "frozen") {
				t.Errorf("note = %q, want the freeze reason", d.Note)
			}
		})
	}
}

func TestPRCommentAudit_RateLimited(t *testing.T) {
	cfg := testPRCommentConfig()
	cfg.PerUserPerMinute = 1
	f := newPRCmdFixture(t, cfg, true)
	f.send(commentOpts{body: "/levelrail status"})
	f.send(commentOpts{body: "/levelrail status"})
	if d := f.wantOneRefusal(prRefusedRateLimitedUser); d.Command != "status" {
		t.Errorf("command = %q, want status", d.Command)
	}
}

func TestPRCommentAudit_ProtectedEnvironment(t *testing.T) {
	f := newPRCmdFixture(t, testPRCommentConfig(), true)
	seedProtectedDeployFixture(t, f.db)
	svc, _ := f.db.GetDesiredService(context.Background(), "web")
	svc.EnvironmentID = "env_prod"
	if err := f.db.SaveDesiredService(context.Background(), *svc); err != nil {
		t.Fatal(err)
	}
	f.send(commentOpts{body: "/levelrail redeploy"})
	if d := f.wantOneRefusal(prRefusedBlocked); !strings.Contains(d.Note, "approval") {
		t.Errorf("note = %q, want the approval reason", d.Note)
	}
}

func TestPRCommentAudit_AcceptedAndNonCommandsNotRefused(t *testing.T) {
	f := newPRCmdFixture(t, testPRCommentConfig(), true)
	f.send(commentOpts{body: "/levelrail status"})
	f.send(commentOpts{body: "looks good to me"})
	f.send(commentOpts{body: "/levelrail redeploy", action: "edited"})
	if got := f.refusals(); len(got) != 0 {
		t.Fatalf("refusal rows = %+v, want none", got)
	}
}
