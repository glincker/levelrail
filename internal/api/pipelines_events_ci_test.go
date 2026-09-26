package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/pipeline"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/webhook"
)

func TestMergeGroupWebhookFiresPipelineEvent(t *testing.T) {
	tests := []struct {
		action    string
		wantFired bool
	}{{"checks_requested", true}, {"destroyed", false}}
	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			rt, _ := newSyncRouter(t, &fakeSyncer{})
			events := &recordingEvents{}
			rt.SetPipelineEvents(events)
			body := `{"action":"` + tt.action + `","merge_group":{"head_sha":"h1","head_ref":"refs/heads/gh-readonly-queue/main/pr-1","base_ref":"refs/heads/main","base_sha":"b1"}}`
			h := http.Header{}
			h.Set("X-GitHub-Event", "merge_group")
			status, _ := rt.processGitPushWebhookPayload(context.Background(), "web", store.GitSource{}, []byte(body), h)
			if status != http.StatusOK {
				t.Fatalf("status = %d", status)
			}
			if fired := len(events.events) == 1; fired != tt.wantFired {
				t.Fatalf("fired = %v, want %v (%+v)", fired, tt.wantFired, events.events)
			}
			if tt.wantFired {
				if ev := events.events[0]; ev.Kind != pipeline.TriggerMergeGroup || ev.Branch != "main" {
					t.Fatalf("event = %+v", ev)
				}
			}
		})
	}
}

func TestPushPipelineEventCarriesChangedFiles(t *testing.T) {
	rt, _ := newSyncRouter(t, &fakeSyncer{})
	events := &recordingEvents{}
	rt.SetPipelineEvents(events)
	rt.triggerPipelinePush(context.Background(), "web", webhook.PushEvent{Ref: "refs/heads/main", After: "a", Before: "b", Changed: []string{"src/a.go"}})
	if len(events.events) != 1 || strings.Join(events.events[0].Changed, ",") != "src/a.go" || events.events[0].ChangedFn == nil {
		t.Fatalf("events = %+v", events.events)
	}
}

func TestPullRequestPipelineEventCarriesAction(t *testing.T) {
	rt, _ := newSyncRouter(t, &fakeSyncer{})
	events := &recordingEvents{}
	rt.SetPipelineEvents(events)
	rt.firePipelinePullRequest(context.Background(), "web", webhook.PullRequestEvent{Action: webhook.PullRequestSynchronize, Number: 4, BaseRef: "main", HeadRef: "f", HeadSHA: "s"})
	if len(events.events) != 1 || events.events[0].Action != "synchronize" || events.events[0].ChangedFn == nil {
		t.Fatalf("events = %+v", events.events)
	}
}

func TestPipelineFiltersEndpoint(t *testing.T) {
	rt, db, _ := newPipelineRouter(t)
	cookie := loginTestSession(t, rt, db)
	yamlText := "version: 1\nname: ci\non:\n  push:\njobs:\n  t:\n    image: a\n    steps:\n      - run: echo\n"
	body := `{"yaml":` + jsonString(yamlText) + `,"paths":["src/**"],"paths_ignore":["docs/**"],"report_status":false}`
	rec := doAuthed(t, rt, cookie, "/api/v1/pipelines/filters", body)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "paths_ignore") || !strings.Contains(rec.Body.String(), "report_status: false") {
		t.Fatalf("filters: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doAuthed(t, rt, cookie, "/api/v1/pipelines/filters", `{"yaml":`+jsonString(yamlText)+`,"paths":["a[b"]}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad glob: %d", rec.Code)
	}
	manual := "version: 1\nname: ci\non:\n  manual:\njobs:\n  t:\n    image: a\n    steps:\n      - run: echo\n"
	if rec := doAuthed(t, rt, cookie, "/api/v1/pipelines/filters", `{"yaml":`+jsonString(manual)+`,"paths":["a"]}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("manual-only pipeline: %d, want 400", rec.Code)
	}
	val := doAuthed(t, rt, cookie, "/api/v1/pipelines/validate", `{"yaml":`+jsonString(yamlText)+`}`)
	if !strings.Contains(val.Body.String(), `"filters":{"paths":[],"paths_ignore":[],"report_status":true}`) {
		t.Fatalf("validate body = %s", val.Body.String())
	}
}

func doAuthed(t *testing.T, rt *Router, cookie *http.Cookie, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, path, body))
	return rec
}
