package api

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestRunDoctorDef(t *testing.T) {
	t.Parallel()
	hang := make(chan struct{})
	t.Cleanup(func() { close(hang) })
	tests := []struct {
		name       string
		def        doctorCheckDef
		wantStatus string
		wantMsg    string
	}{
		{"ok keeps result and stamps group", doctorCheckDef{ID: "a", Group: "host", Timeout: time.Second, Run: func(context.Context) []doctorCheckResource {
			return []doctorCheckResource{{Code: "a", Status: doctorStatusOK}}
		}}, doctorStatusOK, ""},
		{"hang past timeout reports unknown", doctorCheckDef{ID: "slow", Group: "host", Timeout: 20 * time.Millisecond, Run: func(context.Context) []doctorCheckResource {
			<-hang
			return nil
		}}, doctorStatusUnknown, "did not finish"},
		{"panic reports unknown", doctorCheckDef{ID: "boom", Group: "host", Timeout: time.Second, Run: func(context.Context) []doctorCheckResource {
			panic("boom")
		}}, doctorStatusUnknown, "crashed"},
		{"failing check gets a class", doctorCheckDef{ID: "bad", Group: "host", Timeout: time.Second, Run: func(context.Context) []doctorCheckResource {
			return []doctorCheckResource{{Code: "docker", Status: doctorStatusFail, Message: "down"}}
		}}, doctorStatusFail, "down"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res := runDoctorDef(context.Background(), tc.def, slog.Default())
			if len(res) != 1 {
				t.Fatalf("got %d results, want 1", len(res))
			}
			if res[0].Status != tc.wantStatus || !strings.Contains(res[0].Message, tc.wantMsg) {
				t.Errorf("got %q %q, want %q containing %q", res[0].Status, res[0].Message, tc.wantStatus, tc.wantMsg)
			}
			if res[0].Group != "host" {
				t.Errorf("group = %q, want host", res[0].Group)
			}
			if tc.wantStatus == doctorStatusFail && res[0].Class == "" {
				t.Error("failing check has no class")
			}
		})
	}
}

func TestRunDoctorDefs_SlowCheckDoesNotBlockOthers(t *testing.T) {
	t.Parallel()
	rt, _ := newDoctorTestRouter(t)
	hang := make(chan struct{})
	t.Cleanup(func() { close(hang) })
	defs := []doctorCheckDef{
		{ID: "slow", Timeout: 100 * time.Millisecond, Run: func(context.Context) []doctorCheckResource { <-hang; return nil }},
		{ID: "fast", Timeout: time.Second, Run: func(context.Context) []doctorCheckResource {
			return []doctorCheckResource{{Code: "fast", Status: doctorStatusOK}}
		}},
	}
	var order []string
	rt.runDoctorDefs(context.Background(), defs, func(d doctorCheckDef, _ []doctorCheckResource) { order = append(order, d.ID) })
	if len(order) != 2 || order[0] != "fast" {
		t.Errorf("completion order = %v, want fast first then slow", order)
	}
}

func TestDoctorDefs_UniqueIDsAndGroups(t *testing.T) {
	t.Parallel()
	rt, _ := newDoctorTestRouter(t)
	seen := map[string]bool{}
	for _, d := range rt.doctorDefs() {
		if d.ID == "" || d.Group == "" || d.Run == nil {
			t.Errorf("incomplete def %+v", d)
		}
		if seen[d.ID] {
			t.Errorf("duplicate check id %q", d.ID)
		}
		seen[d.ID] = true
	}
}

func TestDoctorRoutes(t *testing.T) {
	rt, db := newDoctorTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	do := func(method, path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, ""))
		return rec
	}
	if rec := do(http.MethodGet, "/api/v1/system/doctor/checks"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"id":"docker"`) {
		t.Errorf("list: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(http.MethodPost, "/api/v1/system/doctor/checks/inodes/run"); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"code":"inodes"`) {
		t.Errorf("run one: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(http.MethodPost, "/api/v1/system/doctor/checks/nope/run"); rec.Code != http.StatusNotFound {
		t.Errorf("run unknown: %d", rec.Code)
	}
	rec := do(http.MethodGet, "/api/v1/system/doctor/stream")
	body := rec.Body.String()
	for _, want := range []string{`"type":"start"`, `"type":"check"`, `"type":"fixes"`, `"type":"done"`} {
		if !strings.Contains(body, want) {
			t.Errorf("stream missing %s", want)
		}
	}
	if !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/event-stream") {
		t.Errorf("content type = %q", rec.Header().Get("Content-Type"))
	}
}

func TestDoctorNodeAgent(t *testing.T) {
	t.Parallel()
	rt, _ := newDoctorTestRouter(t)
	now := time.Now()
	recent, stale := now.Add(-5*time.Second), now.Add(-time.Hour)
	tests := []struct {
		name string
		node store.Node
		want string
	}{
		{"online and fresh", store.Node{ID: "1", Name: "a", Status: store.NodeStatusOnline, LastSeenAt: &recent}, doctorStatusOK},
		{"online but silent", store.Node{ID: "2", Name: "b", Status: store.NodeStatusOnline, LastSeenAt: &stale}, doctorStatusWarn},
		{"never connected", store.Node{ID: "3", Name: "c", Status: store.NodeStatusPending}, doctorStatusWarn},
		{"offline", store.Node{ID: "4", Name: "d", Status: store.NodeStatusOffline, LastSeenAt: &stale}, doctorStatusFail},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := rt.doctorNodeAgent(tc.node, now)
			if c.Status != tc.want {
				t.Errorf("status = %q, want %q (%s)", c.Status, tc.want, c.Message)
			}
			if tc.want != doctorStatusOK && c.Fix == "" {
				t.Error("non-ok node has no fix text")
			}
		})
	}
}
