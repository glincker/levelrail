package api

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// openChangeStream connects as cookie and returns a scanner positioned
// after the connected comment, so publishes from here on are seen.
func openChangeStream(t *testing.T, rt *Router, cookie *http.Cookie) *bufio.Scanner {
	t.Helper()
	srv := httptest.NewServer(rt.Handler())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(func() {
		cancel()
		srv.Close()
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/v1/events/changes", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/v1/events/changes = %d", resp.StatusCode)
	}
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		if sc.Text() == ": connected" {
			return sc
		}
	}
	t.Fatalf("stream ended before connecting: %v", sc.Err())
	return nil
}

func nextChangeTopics(t *testing.T, sc *bufio.Scanner) []string {
	t.Helper()
	ping := false
	for sc.Scan() {
		if strings.HasPrefix(sc.Text(), "event: ") {
			ping = sc.Text() == "event: ping"
			continue
		}
		if data, ok := strings.CutPrefix(sc.Text(), "data: "); ok && ping {
			ping = false
			if !strings.Contains(data, "heartbeat_ms") {
				t.Fatalf("ping without heartbeat: %s", data)
			}
			continue
		} else if ok {
			if strings.Contains(data, appA) || strings.Contains(data, appB) {
				t.Fatalf("change message carries a resource name: %s", data)
			}
			var msg changeMessage
			if err := json.Unmarshal([]byte(data), &msg); err != nil {
				t.Fatalf("decode change message: %v", err)
			}
			return msg.Topics
		}
	}
	t.Fatalf("stream ended before a message: %v", sc.Err())
	return nil
}

// TestChangesStream_TeamScoped: a write in team B never nudges a team A
// subscriber, while its own team's write does, as topic names only.
func TestChangesStream_TeamScoped(t *testing.T) {
	f := newTeamFixture(t)
	for _, role := range []string{store.TeamRoleOwner, store.TeamRoleViewer} {
		sc := openChangeStream(t, f.rt, f.cookies[role])
		f.rt.changes.Publish(changeDomains, appB)
		f.rt.changes.Publish(changeNodes, "node-1")
		f.rt.changes.Publish(changeApps, appA)
		if got := nextChangeTopics(t, sc); !reflect.DeepEqual(got, []string{"apps"}) {
			t.Errorf("%s topics = %v, want [apps]", role, got)
		}
	}
}

// TestOrgIsolation_ChangesStream: an org A member sees nothing of org B
// writes; root sees both topics.
func TestOrgIsolation_ChangesStream(t *testing.T) {
	f := newOrgFixture(t)
	tenant := openChangeStream(t, f.rt, f.cookies[store.TeamRoleOwner])
	root := openChangeStream(t, f.rt, f.root)
	f.rt.changes.Publish(changeAlerts, appB)
	f.rt.changes.Publish(changeDomains, appA)
	if got := nextChangeTopics(t, tenant); !reflect.DeepEqual(got, []string{"domains"}) {
		t.Errorf("org A topics = %v, want [domains]", got)
	}
	got := nextChangeTopics(t, root)
	if len(got) == 1 && got[0] == "alerts" {
		got = append(got, nextChangeTopics(t, root)...)
	}
	if !reflect.DeepEqual(got, []string{"alerts", "domains"}) {
		t.Errorf("root topics = %v, want [alerts domains]", got)
	}
}

// TestChangesStream_WritePublishes: a real domain edit reaches a subscriber.
func TestChangesStream_WritePublishes(t *testing.T) {
	f := newTeamFixture(t)
	sc := openChangeStream(t, f.rt, f.cookies[store.TeamRoleOwner])
	code, body := teamDo(t, f.rt, http.MethodPatch, "/api/v1/apps/"+appA+"/domains",
		`{"add":["a-web.example.test"]}`, withCookie(f.cookies[store.TeamRoleOwner]))
	if code != http.StatusOK {
		t.Fatalf("domain edit = %d %s", code, body)
	}
	if got := nextChangeTopics(t, sc); !reflect.DeepEqual(got, []string{"domains"}) {
		t.Errorf("topics = %v, want [domains]", got)
	}
}

// TestChangesStream_BusCapAndOverflow: the per caller cap holds, a full
// buffer never blocks Publish, and overflow coalesces only for callers that
// may read the whole topic.
func TestChangesStream_BusCapAndOverflow(t *testing.T) {
	var b changeBus
	s, ok := b.subscribe("user:u1", 1, 1)
	if !ok {
		t.Fatal("first subscribe refused")
	}
	if _, ok := b.subscribe("user:u1", 1, 1); ok {
		t.Fatal("second stream for the same caller passed the cap")
	}
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			b.Publish(changeApps, "x")
			b.Publish(changeNodes, "n")
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Publish blocked on a full buffer")
	}
	first := <-s.ch
	open := changeFilter{abilities: []string{AbilityRoot}, scope: &resourceScope{}}
	if got := coalesceChanges(s, open, &first, nil); got != 1<<changeApps|1<<changeNodes {
		t.Errorf("unrestricted mask = %b", got)
	}
	b.Publish(changeApps, "x")
	b.Publish(changeNodes, "n")
	first = <-s.ch
	restricted := changeFilter{abilities: []string{AbilityRead}, scope: &resourceScope{teams: &teamScope{}}}
	if got := coalesceChanges(s, restricted, &first, nil); got&(1<<changeNodes) != 0 {
		t.Errorf("restricted caller got overflowed nodes: %b", got)
	}
	b.unsubscribe(s)
	if _, ok := b.subscribe("user:u1", 1, 1); !ok {
		t.Fatal("cap not released on unsubscribe")
	}
}

// TestChangesStream_NewAppOwnerResolved: an app with no owner row never
// nudges a tenant; once team A owns it, the refreshed filter lets it through.
func TestChangesStream_NewAppOwnerResolved(t *testing.T) {
	f := newTeamFixture(t)
	sc := openChangeStream(t, f.rt, f.cookies[store.TeamRoleOwner])
	f.rt.changes.Publish(changeApps, "c-unowned")
	f.rt.changes.Publish(changeDomains, appA)
	if got := nextChangeTopics(t, sc); !reflect.DeepEqual(got, []string{"domains"}) {
		t.Fatalf("topics = %v, want [domains]", got)
	}
	ctx := context.Background()
	if err := f.db.SaveDesiredService(ctx, store.DesiredService{Name: "a-new", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	if err := f.db.UpdateServiceProject(ctx, "a-new", projA); err != nil {
		t.Fatal(err)
	}
	f.rt.changes.Publish(changeApps, "a-new")
	if got := nextChangeTopics(t, sc); !reflect.DeepEqual(got, []string{"apps"}) {
		t.Fatalf("topics after owner = %v, want [apps]", got)
	}
}

// TestChangesStream_OverflowPolicyAndWake: overflow is withheld from a
// policy bound caller and a full buffer still wakes the reader.
func TestChangesStream_OverflowPolicyAndWake(t *testing.T) {
	var b changeBus
	s, _ := b.subscribe("token:t1", 1, 1)
	b.Publish(changeApps, "x")
	b.Publish(changeApps, "y")
	select {
	case <-s.wake:
	default:
		t.Fatal("overflow did not wake the reader")
	}
	first := <-s.ch
	policyBound := changeFilter{abilities: []string{AbilityRoot}, policies: []store.Policy{{}}, scope: &resourceScope{}}
	b.Publish(changeNodes, "n")
	b.Publish(changeNodes, "m")
	if got := coalesceChanges(s, policyBound, &first, nil); got&(1<<changeNodes) != 0 {
		t.Errorf("policy bound caller got overflowed nodes: %b", got)
	}
}

func TestChangesStream_HeartbeatClamped(t *testing.T) {
	for in, want := range map[time.Duration]time.Duration{
		0: 20 * time.Second, time.Second: changesHeartbeatFloor, time.Hour: changesHeartbeatCeil, 30 * time.Second: 30 * time.Second,
	} {
		if got := clampHeartbeat(in); got != want {
			t.Errorf("clampHeartbeat(%s) = %s, want %s", in, got, want)
		}
	}
}
