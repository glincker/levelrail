package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/GLINCKER/levelrail/internal/plugin/pluginpb"
	"github.com/GLINCKER/levelrail/plugins/internal/pluginkit"
	"github.com/GLINCKER/levelrail/plugins/internal/pluginkit/pluginkittest"
)

const testToken = "cf-token-SECRET-123" //nolint:gosec // test fixture, not a credential

type fakeCF struct {
	mu       sync.Mutex
	records  []cfRecord
	dropPost bool
	echoAuth bool
	next     int
}

func (f *fakeCF) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer "+testToken {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	reply := func(result any) {
		b, _ := json.Marshal(map[string]any{"success": true, "result": result, "result_info": map[string]int{"total_pages": 1}})
		_, _ = w.Write(b)
	}
	p := r.URL.Path
	switch {
	case p == "/zones":
		if n := r.URL.Query().Get("name"); n != "" && n != "example.com" {
			reply([]any{})
			return
		}
		reply([]map[string]string{{"id": "z1", "name": "example.com"}})
	case p == "/zones/z1/dns_records" && r.Method == http.MethodGet:
		var out []cfRecord
		for _, x := range f.records {
			if t := r.URL.Query().Get("type"); t != "" && t != x.Type {
				continue
			}
			if n := r.URL.Query().Get("name"); n != "" && n != x.Name {
				continue
			}
			out = append(out, x)
		}
		reply(out)
	case p == "/zones/z1/dns_records" && r.Method == http.MethodPost:
		var rec cfRecord
		_ = json.NewDecoder(r.Body).Decode(&rec)
		if f.echoAuth {
			b, _ := json.Marshal(map[string]any{"success": false, "errors": []map[string]string{{"message": "bad request for " + testToken}}})
			_, _ = w.Write(b)
			return
		}
		f.next++
		rec.ID = "r" + strconv.Itoa(f.next)
		if !f.dropPost {
			f.records = append(f.records, rec)
		}
		reply(rec)
	case strings.HasPrefix(p, "/zones/z1/dns_records/") && r.Method == http.MethodPut:
		var rec cfRecord
		_ = json.NewDecoder(r.Body).Decode(&rec)
		id := strings.TrimPrefix(p, "/zones/z1/dns_records/")
		for i := range f.records {
			if f.records[i].ID == id {
				rec.ID = id
				f.records[i] = rec
			}
		}
		reply(rec)
	case strings.HasPrefix(p, "/zones/z1/dns_records/") && r.Method == http.MethodDelete:
		id := strings.TrimPrefix(p, "/zones/z1/dns_records/")
		kept := f.records[:0]
		for _, x := range f.records {
			if x.ID != id {
				kept = append(kept, x)
			}
		}
		f.records = kept
		reply(map[string]string{"id": id})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func setup(t *testing.T, f *fakeCF) {
	t.Helper()
	srv := httptest.NewServer(f)
	old := apiBase
	apiBase = srv.URL
	t.Cleanup(func() { apiBase = old; srv.Close() })
}

func rec(name, value string) *pluginpb.DnsRecord {
	return &pluginpb.DnsRecord{Name: name, Type: "A", Value: value, TtlSeconds: 1}
}

func TestContract(t *testing.T) {
	pluginkittest.CheckDNSContract(t, newPlugin(), "plugin.yaml")
}

func TestZonesRecordsRoundTrip(t *testing.T) {
	f := &fakeCF{}
	setup(t, f)
	ctx := context.Background()
	c := pluginkit.DNSCall{Secret: testToken, Zone: "example.com.", Record: rec("app", "203.0.113.5")}

	zones, o := listZones(ctx, pluginkit.DNSCall{Secret: testToken})
	if !o.OK || len(zones) != 1 || zones[0] != "example.com." {
		t.Fatalf("zones = %v %+v", zones, o)
	}
	if o := upsert(ctx, c); !o.OK {
		t.Fatalf("upsert: %+v", o)
	}
	if ok, o := verify(ctx, c); !o.OK || !ok {
		t.Fatalf("verify after create: %v %+v", ok, o)
	}
	c.Record = rec("app", "203.0.113.9")
	if o := upsert(ctx, c); !o.OK {
		t.Fatalf("upsert update: %+v", o)
	}
	if len(f.records) != 1 || f.records[0].Content != "203.0.113.9" {
		t.Fatalf("update must replace in place: %+v", f.records)
	}
	rs, o := listRecords(ctx, c)
	if !o.OK || len(rs) != 1 || rs[0].GetName() != "app" {
		t.Fatalf("list = %v %+v", rs, o)
	}
	if o := remove(ctx, c); !o.OK || len(f.records) != 0 {
		t.Fatalf("remove: %+v %+v", o, f.records)
	}
}

func TestRemoveLeavesDifferentValue(t *testing.T) {
	f := &fakeCF{records: []cfRecord{{ID: "r9", Type: "A", Name: "app.example.com", Content: "198.51.100.1"}}}
	setup(t, f)
	c := pluginkit.DNSCall{Secret: testToken, Zone: "example.com.", Record: rec("app", "203.0.113.5")}
	if o := remove(context.Background(), c); !o.OK || len(f.records) != 1 {
		t.Fatalf("a record someone changed must stay: %+v %+v", o, f.records)
	}
}

func TestHalfSuccessVerifyReportsMissing(t *testing.T) {
	f := &fakeCF{dropPost: true}
	setup(t, f)
	c := pluginkit.DNSCall{Secret: testToken, Zone: "example.com.", Record: rec("app", "203.0.113.5")}
	if o := upsert(context.Background(), c); !o.OK {
		t.Fatalf("upsert: %+v", o)
	}
	ok, o := verify(context.Background(), c)
	if !o.OK || ok {
		t.Fatalf("verify must say the record is absent: %v %+v", ok, o)
	}
}

func TestErrorsNeverCarryTheToken(t *testing.T) {
	f := &fakeCF{echoAuth: true}
	setup(t, f)
	c := pluginkit.DNSCall{Secret: testToken, Zone: "example.com.", Record: rec("app", "203.0.113.5")}
	o := upsert(context.Background(), c)
	if o.OK || strings.Contains(o.Detail, testToken) {
		t.Fatalf("got %+v", o)
	}
	bad := pluginkit.DNSCall{Secret: "wrong", Zone: "example.com."}
	_, o = listZones(context.Background(), bad)
	if o.OK || o.Retryable || !strings.Contains(o.Detail, "authentication failed") {
		t.Fatalf("got %+v", o)
	}
	_, o = listZones(context.Background(), pluginkit.DNSCall{})
	if o.OK {
		t.Fatal("a missing token must be refused")
	}
}

func TestUnknownZone(t *testing.T) {
	setup(t, &fakeCF{})
	c := pluginkit.DNSCall{Secret: testToken, Zone: "other.org.", Record: rec("a", "203.0.113.5")}
	if o := upsert(context.Background(), c); o.OK || !strings.Contains(o.Detail, "not found") {
		t.Fatalf("got %+v", o)
	}
}

func TestRetryableMapping(t *testing.T) {
	for code, retry := range map[int]bool{429: true, 503: true} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(code) }))
		old := apiBase
		apiBase = srv.URL
		_, o := listZones(context.Background(), pluginkit.DNSCall{Secret: testToken})
		apiBase = old
		srv.Close()
		if o.OK || o.Retryable != retry {
			t.Fatalf("HTTP %d: %+v", code, o)
		}
	}
}
