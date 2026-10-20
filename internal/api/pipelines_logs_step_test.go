package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestListPipelineRunLogs_StepFilterAppliesBeforeLimit(t *testing.T) {
	rt, db, _ := newPipelineRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")
	ctx := context.Background()
	now := time.Now().UTC()
	p, _ := db.SavePipeline(ctx, store.Pipeline{ID: "p1", AppName: "web", Name: "ci", YAML: apiPipelineYAML, Enabled: true, CreatedAt: now, UpdatedAt: now})
	_, _ = db.CreatePipelineRun(ctx, store.PipelineRun{ID: "r1", PipelineID: p.ID, AppName: "web", TriggerKind: "manual", Definition: apiPipelineYAML, CreatedAt: now})
	var lines []store.PipelineLogLine
	for i := 0; i < 5; i++ {
		lines = append(lines, store.PipelineLogLine{RunID: "r1", JobKey: "t", StepIndex: 0, Stream: "stdout", Line: fmt.Sprintf("early %d", i), CreatedAt: now})
	}
	lines = append(lines, store.PipelineLogLine{RunID: "r1", JobKey: "t", StepIndex: 1, Stream: "stdout", Line: "late 0", CreatedAt: now})
	lines = append(lines, store.PipelineLogLine{RunID: "r1", JobKey: "t", StepIndex: 1, Stream: "stdout", Line: "late 1", CreatedAt: now})
	if err := db.AppendPipelineLogs(ctx, lines); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		query    string
		wantCode int
		want     []string
	}{
		{"limit crowds out late step without filter", "limit=3", http.StatusOK, []string{"early 0", "early 1", "early 2"}},
		{"step filter applies before limit", "limit=3&step=1", http.StatusOK, []string{"late 0", "late 1"}},
		{"step with pagination cursor", "limit=1&step=1&after=6", http.StatusOK, []string{"late 1"}},
		{"step zero is a real step", "limit=2&step=0", http.StatusOK, []string{"early 0", "early 1"}},
		{"invalid step", "step=abc", http.StatusBadRequest, nil},
		{"negative step", "step=-1", http.StatusBadRequest, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/pipeline-runs/r1/logs?job=t&"+tt.query, ""))
			if rec.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, tt.wantCode, rec.Body.String())
			}
			if tt.wantCode != http.StatusOK {
				return
			}
			var got []pipelineLogView
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d lines %+v, want %v", len(got), got, tt.want)
			}
			for i, w := range tt.want {
				if got[i].Line != w {
					t.Fatalf("line %d = %q, want %q", i, got[i].Line, w)
				}
			}
		})
	}
}
