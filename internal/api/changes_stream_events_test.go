package api

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func nextChangeData(t *testing.T, sc *bufio.Scanner) string {
	t.Helper()
	skip := false
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "event: ") {
			skip = true
			continue
		}
		if data, ok := strings.CutPrefix(line, "data: "); ok {
			if skip {
				skip = false
				continue
			}
			return data
		}
	}
	t.Fatalf("stream ended before a message: %v", sc.Err())
	return ""
}

// TestChangesStreamEvents_ScopedPerCaller: a team A caller receives the
// deploy and alert payloads of its own app and none for team B's.
func TestChangesStreamEvents_ScopedPerCaller(t *testing.T) {
	f := newTeamFixture(t)
	sc := openChangeStream(t, f.rt, f.cookies[store.TeamRoleViewer])
	f.rt.changes.PublishDetail(changeApps, appB, "apps.deploy", 200)
	f.rt.changes.PublishDetail(changeAlerts, appB, "apps.alerts", 201)
	f.rt.changes.PublishDetail(changeApps, appA, "apps.deploy", 200)
	data := nextChangeData(t, sc)
	if strings.Contains(data, appB) {
		t.Fatalf("payload leaks another team's app: %s", data)
	}
	var msg changeMessage
	if err := json.Unmarshal([]byte(data), &msg); err != nil {
		t.Fatal(err)
	}
	want := []changeDetail{{Type: "apps.deploy", ResourceID: appA, Status: 200}}
	if !reflect.DeepEqual(msg.Events, want) || !reflect.DeepEqual(msg.Topics, []string{"apps"}) {
		t.Fatalf("got topics=%v events=%+v, want apps and %+v", msg.Topics, msg.Events, want)
	}
}

// TestChangesStreamEvents_NoReadAbilityGetsNothing: a caller without the
// read ability receives neither a nudge nor a payload.
func TestChangesStreamEvents_NoReadAbilityGetsNothing(t *testing.T) {
	var b changeBus
	s, _ := b.subscribe("token:t1", 4, 1)
	b.PublishDetail(changeApps, appA, "apps.deploy", 200)
	first := <-s.ch
	none := changeFilter{abilities: []string{}, scope: &resourceScope{}}
	if mask, ev := coalesceDetails(s, none, &first, nil); mask != 0 || len(ev) != 0 {
		t.Fatalf("no-ability caller got mask=%b events=%+v", mask, ev)
	}
	b.PublishDetail(changeApps, appA, "apps.deploy", 200)
	first = <-s.ch
	read := changeFilter{abilities: []string{AbilityRead}, scope: &resourceScope{}}
	if _, ev := coalesceDetails(s, read, &first, nil); len(ev) != 1 {
		t.Fatalf("read caller events = %+v, want 1", ev)
	}
}

// TestChangesStreamEvents_RevokedSessionRejected: once a credential is
// revoked the stream cannot be opened with it.
func TestChangesStreamEvents_RevokedSessionRejected(t *testing.T) {
	f := newTeamFixture(t)
	c := f.cookies[store.TeamRoleViewer]
	out := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	out.AddCookie(c)
	f.rt.Handler().ServeHTTP(httptest.NewRecorder(), out)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events/changes", nil)
	req.AddCookie(c)
	rec := httptest.NewRecorder()
	f.rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("revoked credential got %d, want 401", rec.Code)
	}
}

// TestChangesStreamEvents_PayloadFieldsAreIDsAndStatus: the payload type
// has no room for env values or secrets, and an audit entry's body never
// reaches it.
func TestChangesStreamEvents_PayloadFieldsAreIDsAndStatus(t *testing.T) {
	raw, _ := json.Marshal(changeDetail{Type: "apps.deploy", ResourceID: appA, Status: 200})
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	keys := []string{}
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if !reflect.DeepEqual(keys, []string{"resource_id", "status", "type"}) {
		t.Fatalf("payload keys = %v", keys)
	}
	f := newTeamFixture(t)
	sc := openChangeStream(t, f.rt, f.cookies[store.TeamRoleOwner])
	f.rt.publishStreamEvent("apps.deploy", store.AuditEntry{
		Method: http.MethodPost, Path: "/api/v1/apps/" + appA + "/deploy", StatusCode: 200,
		ActorName: "tester", Detail: "API_KEY=sk_live_secretvalue",
	})
	data := nextChangeData(t, sc)
	if strings.Contains(data, "secretvalue") || strings.Contains(data, "tester") {
		t.Fatalf("payload carries audit detail: %s", data)
	}
}
