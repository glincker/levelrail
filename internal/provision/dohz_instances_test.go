package provision

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type dohzCalls struct {
	mu   sync.Mutex
	rows []string
}

func (c *dohzCalls) add(r *http.Request, body string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rows = append(c.rows, r.Method+" "+r.URL.Path+" "+body)
}

func (c *dohzCalls) has(s string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, r := range c.rows {
		if strings.Contains(r, s) {
			return true
		}
	}
	return false
}

func dohzServer(t *testing.T, calls *dohzCalls, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		calls.add(r, string(b))
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestInstanceDO_CapabilitiesAreHonest(t *testing.T) {
	m := (&DigitalOcean{}).InstanceCapabilities()
	if m.Supports(VerbKeepBootVol) {
		t.Fatal("droplet disks cannot be kept")
	}
	if !m.SupportsAction(InstanceActionSoftReboot) || !m.Supports(VerbResizeByShape) {
		t.Fatalf("matrix = %v", m.Verbs())
	}
}

func TestInstanceDO_ListDescribeAndReadOnly(t *testing.T) {
	calls := &dohzCalls{}
	droplet := func(id int64, tags []string) map[string]any {
		return map[string]any{"id": id, "name": "n", "status": "off", "size_slug": "s-1vcpu-1gb", "vcpus": 1, "memory": 1024,
			"disk": 25, "created_at": "2026-01-02T03:04:05Z", "tags": tags, "region": map[string]any{"slug": "fra1"},
			"networks": map[string]any{"v4": []map[string]any{{"ip_address": "1.2.3.4", "type": "public"}}}}
	}
	srv := dohzServer(t, calls, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/droplets":
			_ = json.NewEncoder(w).Encode(map[string]any{"droplets": []any{droplet(1, []string{"levelrail", "env:prod"}), droplet(2, nil)}})
		case "/droplets/2":
			_ = json.NewEncoder(w).Encode(map[string]any{"droplet": droplet(2, nil)})
		default:
			w.WriteHeader(http.StatusOK)
		}
	})
	d := &DigitalOcean{client: newHTTPClient(srv.URL, "t")}
	rows, err := d.ListInstances(context.Background(), InstanceScope{})
	if err != nil || len(rows) != 2 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	if !rows[0].Managed || rows[0].Tags["env"] != "prod" || rows[0].State != InstanceStateStopped || rows[0].MemoryGB != 1 || rows[1].Managed {
		t.Fatalf("row = %+v", rows[0])
	}
	if err := d.InstanceAction(context.Background(), "2", InstanceActionStart); !errors.Is(err, ErrInstanceReadOnly) {
		t.Fatalf("unmanaged action err = %v", err)
	}
	if calls.has("POST /droplets/2/actions") {
		t.Fatal("no write may reach an unmanaged droplet")
	}
	if _, err := d.DescribeInstance(context.Background(), "../x"); !errors.Is(err, ErrInstanceInvalid) {
		t.Fatalf("bad id err = %v", err)
	}
}

func TestInstanceDO_ActionsResizeTagsTerminate(t *testing.T) {
	calls := &dohzCalls{}
	srv := dohzServer(t, calls, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/droplets/1" {
			_ = json.NewEncoder(w).Encode(map[string]any{"droplet": map[string]any{"id": 1, "status": "active", "tags": []string{"levelrail", "env:dev"}}})
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/droplets/1/actions" {
			b, _ := io.ReadAll(r.Body)
			_ = b
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	})
	d := &DigitalOcean{client: newHTTPClient(srv.URL, "t")}
	ctx := context.Background()
	if err := d.InstanceAction(ctx, "1", InstanceActionSoftStop); err != nil || !calls.has(`"type":"shutdown"`) {
		t.Fatalf("soft stop err=%v", err)
	}
	if err := d.ResizeInstance(ctx, "1", ResizeSpec{InstanceType: "s-2vcpu-2gb"}); err != nil || !calls.has(`"disk":false`) {
		t.Fatalf("resize err=%v", err)
	}
	if err := d.SetInstanceTags(ctx, "1", map[string]string{"env": "prod"}); err != nil {
		t.Fatalf("tags err=%v", err)
	}
	if !calls.has("DELETE /tags/env:dev/resources") || !calls.has("POST /tags/env:prod/resources") {
		t.Fatalf("calls = %v", calls.rows)
	}
	if err := d.SetInstanceTags(ctx, "1", map[string]string{"levelrail": "x"}); !errors.Is(err, ErrInstanceInvalid) {
		t.Fatalf("ownership tag err = %v", err)
	}
	if err := d.TerminateInstance(ctx, "1", true); !errors.Is(err, ErrInstanceUnsupported) {
		t.Fatalf("keep disk err = %v", err)
	}
	if err := d.TerminateInstance(ctx, "1", false); err != nil || !calls.has("DELETE /droplets/1 ") {
		t.Fatalf("terminate err=%v", err)
	}
}

func TestInstanceDO_EventsAndConflict(t *testing.T) {
	calls := &dohzCalls{}
	srv := dohzServer(t, calls, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/actions") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"actions": []any{
				map[string]any{"type": "reboot", "status": "completed", "started_at": "2026-05-01T00:00:00Z"},
				map[string]any{"type": "old", "status": "completed", "started_at": "2020-05-01T00:00:00Z"},
			}})
		case r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"droplet": map[string]any{"id": 1, "tags": []string{"levelrail"}}})
		default:
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"message":"not off"}`))
		}
	})
	d := &DigitalOcean{client: newHTTPClient(srv.URL, "t")}
	ev, err := d.InstanceEvents(context.Background(), "1", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || len(ev) != 1 || ev[0].Kind != "reboot" {
		t.Fatalf("events=%v err=%v", ev, err)
	}
	if err := d.ResizeInstance(context.Background(), "1", ResizeSpec{InstanceType: "x"}); !errors.Is(err, ErrInstanceConflict) {
		t.Fatalf("resize err = %v", err)
	}
}

func TestInstanceHetzner_ListActionsLabelsAndReadOnly(t *testing.T) {
	calls := &dohzCalls{}
	server := func(id int64, labels map[string]string) map[string]any {
		return map[string]any{"id": id, "name": "h", "status": "running", "created": "2026-01-02T03:04:05Z", "labels": labels,
			"server_type": map[string]any{"name": "cx22", "cores": 2, "memory": 4, "disk": 40},
			"datacenter":  map[string]any{"name": "fsn1-dc14", "location": map[string]any{"name": "fsn1"}},
			"public_net":  map[string]any{"ipv4": map[string]any{"ip": "5.6.7.8"}}}
	}
	srv := dohzServer(t, calls, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/servers":
			_ = json.NewEncoder(w).Encode(map[string]any{"servers": []any{server(1, map[string]string{"levelrail": "true", "a": "b"}), server(2, nil)}})
		case r.Method == http.MethodGet && r.URL.Path == "/servers/1":
			_ = json.NewEncoder(w).Encode(map[string]any{"server": server(1, map[string]string{"levelrail": "true", "a": "b"})})
		case r.Method == http.MethodGet && r.URL.Path == "/servers/2":
			_ = json.NewEncoder(w).Encode(map[string]any{"server": server(2, nil)})
		case strings.HasSuffix(r.URL.Path, "/actions") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"actions": []any{map[string]any{"command": "reset_server", "status": "success", "started": "2026-05-01T00:00:00Z"}}})
		default:
			_, _ = w.Write([]byte("{}"))
		}
	})
	h := &Hetzner{client: newHTTPClient(srv.URL, "t")}
	ctx := context.Background()
	rows, err := h.ListInstances(ctx, InstanceScope{Region: "fsn1"})
	if err != nil || len(rows) != 2 || !rows[0].Managed || rows[1].Managed || rows[0].OCPUs != 2 || rows[0].MemoryGB != 4 {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
	if err := h.InstanceAction(ctx, "2", InstanceActionStop); !errors.Is(err, ErrInstanceReadOnly) {
		t.Fatalf("unmanaged err = %v", err)
	}
	if err := h.InstanceAction(ctx, "1", InstanceActionReboot); err != nil || !calls.has("POST /servers/1/actions/reset") {
		t.Fatalf("reboot err=%v", err)
	}
	if err := h.InstanceAction(ctx, "1", InstanceActionSoftReboot); err != nil || !calls.has("POST /servers/1/actions/reboot") {
		t.Fatalf("soft reboot err=%v", err)
	}
	if err := h.ResizeInstance(ctx, "1", ResizeSpec{InstanceType: "cx32"}); err != nil || !calls.has(`"upgrade_disk":false`) {
		t.Fatalf("resize err=%v", err)
	}
	if err := h.SetInstanceTags(ctx, "1", map[string]string{"a": "", "c": "d"}); err != nil {
		t.Fatalf("tags err=%v", err)
	}
	if !calls.has(`"labels":{"c":"d","levelrail":"true"}`) {
		t.Fatalf("calls = %v", calls.rows)
	}
	if err := h.SetInstanceTags(ctx, "1", map[string]string{"levelrail": ""}); !errors.Is(err, ErrInstanceInvalid) {
		t.Fatalf("ownership err = %v", err)
	}
	if err := h.TerminateInstance(ctx, "1", true); !errors.Is(err, ErrInstanceUnsupported) {
		t.Fatalf("keep disk err = %v", err)
	}
	ev, err := h.InstanceEvents(ctx, "1", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || len(ev) != 1 {
		t.Fatalf("events=%v err=%v", ev, err)
	}
	if (&Hetzner{}).InstanceCapabilities().Supports(VerbKeepBootVol) {
		t.Fatal("keep boot volume must be false")
	}
}
