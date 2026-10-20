package dnszones

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
)

// cfFake replays recorded Cloudflare API shapes and keeps records in memory.
type cfFake struct {
	mu      sync.Mutex
	records []map[string]any
	nextID  int
	log     []string
	bodies  []map[string]any
}

func (f *cfFake) ok(w http.ResponseWriter, result any, info bool) {
	env := map[string]any{"success": true, "errors": []any{}, "result": result}
	if info {
		env["result_info"] = map[string]any{"page": 1, "total_pages": 1}
	}
	_ = json.NewEncoder(w).Encode(env)
}

func (f *cfFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer tok" {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"success":false,"errors":[{"code":10000,"message":"Authentication error"}]}`)
		return
	}
	f.log = append(f.log, r.Method+" "+r.URL.Path)
	var body map[string]any
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body != nil {
			f.bodies = append(f.bodies, body)
		}
	}
	zone := map[string]any{"id": "z1", "name": "example.com", "status": "pending", "name_servers": []string{"ana.ns.cloudflare.com", "bob.ns.cloudflare.com"}, "account": map[string]string{"id": "acct1"}, "modified_on": "2026-10-01T00:00:00Z"}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/zones":
		f.ok(w, []any{zone}, true)
	case r.Method == http.MethodGet && r.URL.Path == "/zones/z1":
		f.ok(w, zone, false)
	case r.Method == http.MethodPost && r.URL.Path == "/zones":
		f.ok(w, zone, false)
	case r.Method == http.MethodDelete && r.URL.Path == "/zones/z1":
		f.ok(w, map[string]string{"id": "z1"}, false)
	case r.Method == http.MethodGet && r.URL.Path == "/zones/z1/dns_records":
		q := r.URL.Query()
		out := []map[string]any{}
		for _, rec := range f.records {
			if (q.Get("type") == "" || rec["type"] == q.Get("type")) && (q.Get("name") == "" || rec["name"] == q.Get("name")) {
				out = append(out, rec)
			}
		}
		f.ok(w, out, true)
	case r.Method == http.MethodPost && r.URL.Path == "/zones/z1/dns_records":
		f.nextID++
		body["id"] = fmt.Sprintf("r%d", f.nextID)
		f.records = append(f.records, body)
		f.ok(w, body, false)
	case strings.HasPrefix(r.URL.Path, "/zones/z1/dns_records/"):
		id := strings.TrimPrefix(r.URL.Path, "/zones/z1/dns_records/")
		i := slices.IndexFunc(f.records, func(m map[string]any) bool { return m["id"] == id })
		if i < 0 {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"success":false,"errors":[{"code":81044,"message":"Record does not exist."}]}`)
			return
		}
		if r.Method == http.MethodDelete {
			f.records = slices.Delete(f.records, i, i+1)
		} else {
			for k, v := range body {
				f.records[i][k] = v
			}
		}
		f.ok(w, map[string]string{"id": id}, false)
	default:
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"success":false,"errors":[{"code":7003,"message":"No route"}]}`)
	}
}

func newCFTest(t *testing.T) (*Cloudflare, *cfFake) {
	t.Helper()
	fake := &cfFake{}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	c := NewCloudflare("tok")
	c.BaseURL = srv.URL
	return c, fake
}

func TestCloudflareZones(t *testing.T) {
	c, fake := newCFTest(t)
	ctx := context.Background()
	zones, err := c.ListZones(ctx)
	if err != nil || len(zones) != 1 || zones[0].Name != "example.com" || len(zones[0].NameServers) != 2 || zones[0].RecordCount != nil {
		t.Fatalf("zones = %+v, err = %v", zones, err)
	}
	z, err := c.CreateZone(ctx, "Example.com.")
	if err != nil || z.ID != "z1" {
		t.Fatalf("create: %+v %v", z, err)
	}
	last := fake.bodies[len(fake.bodies)-1]
	if last["name"] != "example.com" || last["type"] != "full" || last["account"].(map[string]any)["id"] != "acct1" {
		t.Errorf("create body = %v", last)
	}
	if err := c.DeleteZone(ctx, "z1"); err != nil {
		t.Fatal(err)
	}
	bad := NewCloudflare("wrong")
	bad.BaseURL = c.BaseURL
	_, err = bad.ListZones(ctx)
	if err == nil || strings.Contains(err.Error(), "wrong") || !strings.Contains(err.Error(), "Authentication error") {
		t.Errorf("auth error = %v (must not echo the token)", err)
	}
}

func TestCloudflareRecordSets(t *testing.T) {
	c, fake := newCFTest(t)
	ctx := context.Background()
	zone := Zone{ID: "z1", Name: "example.com"}
	sets := []RecordSet{
		{Name: "www", Type: "A", TTL: 300, Proxied: true, Values: []string{"1.1.1.1", "2.2.2.2"}},
		{Name: Apex, Type: "MX", TTL: 300, Values: []string{"10 mail.example.com"}},
		{Name: Apex, Type: "TXT", TTL: 300, Values: []string{"v=spf1 mx ~all"}},
		{Name: Apex, Type: "CAA", TTL: 300, Values: []string{`0 issuewild "letsencrypt.org"`}},
		{Name: "_sip._tls", Type: "SRV", TTL: 300, Values: []string{"100 1 443 sip.example.net"}},
	}
	for _, s := range sets {
		if err := c.UpsertRecordSet(ctx, zone, s); err != nil {
			t.Fatalf("upsert %s: %v", s.Type, err)
		}
	}
	got, err := c.ListRecordSets(ctx, zone)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range sets {
		i := slices.IndexFunc(got, func(g RecordSet) bool { return g.Key() == want.Key() })
		if i < 0 || !SameContent(got[i], want) {
			t.Errorf("%s %s: got %+v", want.Name, want.Type, got)
		}
	}
	txt := slices.IndexFunc(fake.records, func(m map[string]any) bool { return m["type"] == "TXT" })
	if fake.records[txt]["content"] != `"v=spf1 mx ~all"` {
		t.Errorf("TXT content must be quoted, got %v", fake.records[txt]["content"])
	}

	before := len(fake.log)
	if err := c.UpsertRecordSet(ctx, zone, RecordSet{Name: "www", Type: "A", TTL: 300, Proxied: true, Values: []string{"2.2.2.2", "3.3.3.3"}}); err != nil {
		t.Fatal(err)
	}
	ops := strings.Join(fake.log[before:], ",")
	if strings.Count(ops, "POST") != 1 || strings.Count(ops, "DELETE") != 1 || strings.Contains(ops, "PATCH") {
		t.Errorf("upsert should add one and delete one, ops = %s", ops)
	}
	if err := c.UpsertRecordSet(ctx, zone, RecordSet{Name: "www", Type: "A", TTL: 600, Values: []string{"2.2.2.2", "3.3.3.3"}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(fake.log, ","), "PATCH") {
		t.Error("ttl change should patch in place")
	}
	if err := c.DeleteRecordSet(ctx, zone, Key{Name: "www", Type: "A"}); err != nil {
		t.Fatal(err)
	}
	got, _ = c.ListRecordSets(ctx, zone)
	if slices.ContainsFunc(got, func(g RecordSet) bool { return g.Name == "www" }) {
		t.Error("www should be gone")
	}
}
