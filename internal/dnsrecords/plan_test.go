package dnsrecords

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/libdns/libdns"
)

func rr(name, typ, data string) libdns.Record {
	return libdns.RR{Name: name, Type: typ, Data: data, TTL: time.Minute}
}

func TestPlanRecord(t *testing.T) {
	t.Parallel()
	aWant := Desired{Name: "app", Type: "A", Value: "203.0.113.5"}
	tests := []struct {
		name     string
		desired  Desired
		existing []libdns.Record
		replace  bool
		want     string
		replaced int
	}{
		{"create on empty zone", aWant, nil, false, OutcomeCreated, 0},
		{"identical is unchanged", aWant, []libdns.Record{rr("app", "A", "203.0.113.5")}, false, OutcomeUnchanged, 0},
		{"identical next to a sibling is unchanged", aWant, []libdns.Record{rr("app", "A", "198.51.100.1"), rr("app", "A", "203.0.113.5")}, false, OutcomeUnchanged, 0},
		{"different A is a conflict", aWant, []libdns.Record{rr("app", "A", "198.51.100.1")}, false, OutcomeConflict, 1},
		{"different A with replace updates", aWant, []libdns.Record{rr("app", "A", "198.51.100.1")}, true, OutcomeUpdated, 1},
		{"CNAME blocks an A", aWant, []libdns.Record{rr("app", "CNAME", "other.example.net.")}, false, OutcomeConflict, 1},
		{"AAAA coexists with an A", aWant, []libdns.Record{rr("app", "AAAA", "2001:db8::1")}, false, OutcomeCreated, 0},
		{"TXT is never blocking", aWant, []libdns.Record{rr("app", "TXT", "v=spf1")}, true, OutcomeCreated, 0},
		{"other name is ignored", aWant, []libdns.Record{rr("www", "A", "198.51.100.1")}, false, OutcomeCreated, 0},
		{"ipv6 only host", Desired{Name: "app", Type: "AAAA", Value: "2001:db8::5"}, []libdns.Record{rr("app", "A", "198.51.100.1")}, false, OutcomeCreated, 0},
		{"cname conflicts with A and AAAA", Desired{Name: "app", Type: "CNAME", Value: "edge.example.net"}, []libdns.Record{rr("app", "A", "198.51.100.1"), rr("app", "AAAA", "2001:db8::1")}, false, OutcomeConflict, 2},
		{"cname trailing dot is unchanged", Desired{Name: "app", Type: "CNAME", Value: "edge.example.net"}, []libdns.Record{rr("app", "CNAME", "edge.example.net.")}, false, OutcomeUnchanged, 0},
		{"apex name matches", Desired{Name: "@", Type: "A", Value: "203.0.113.5"}, []libdns.Record{rr("@", "A", "203.0.113.5")}, false, OutcomeUnchanged, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := PlanRecord(tc.desired, tc.existing, tc.replace)
			if got.Outcome != tc.want || len(got.Replace) != tc.replaced {
				t.Fatalf("PlanRecord = %s with %d replace, want %s with %d", got.Outcome, len(got.Replace), tc.want, tc.replaced)
			}
		})
	}
}

func TestTargetFor(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		ips       []string
		wantType  string
		wantValue string
		wantErr   bool
	}{
		{"ipv4 wins", []string{"2001:db8::1", "203.0.113.9"}, "A", "203.0.113.9", false},
		{"ipv6 only", []string{"2001:db8::1"}, "AAAA", "2001:db8::1", false},
		{"hostnames ignored", []string{"host.example.net"}, "", "", true},
		{"empty", nil, "", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			typ, val, err := TargetFor(tc.ips)
			if (err != nil) != tc.wantErr || typ != tc.wantType || val != tc.wantValue {
				t.Fatalf("TargetFor = %q %q %v", typ, val, err)
			}
		})
	}
}

func TestMatchZoneAndRelativeName(t *testing.T) {
	t.Parallel()
	zones := []string{"example.com.", "dev.example.com.", "example.co.uk"}
	tests := []struct {
		domain   string
		wantZone string
		wantOK   bool
		wantName string
	}{
		{"example.com", "example.com.", true, "@"},
		{"app.example.com", "example.com.", true, "app"},
		{"a.b.example.com", "example.com.", true, "a.b"},
		{"app.dev.example.com", "dev.example.com.", true, "app"},
		{"dev.example.com", "dev.example.com.", true, "@"},
		{"app.example.co.uk", "example.co.uk.", true, "app"},
		{"other.org", "", false, ""},
		{"com", "", false, ""},
	}
	for _, tc := range tests {
		t.Run(tc.domain, func(t *testing.T) {
			t.Parallel()
			zone, ok := MatchZone(tc.domain, zones)
			if zone != tc.wantZone || ok != tc.wantOK {
				t.Fatalf("MatchZone = %q %v", zone, ok)
			}
			if ok {
				if got := RelativeName(tc.domain, zone); got != tc.wantName {
					t.Fatalf("RelativeName = %q, want %q", got, tc.wantName)
				}
			}
		})
	}
}

func TestValidateTarget(t *testing.T) {
	t.Parallel()
	if err := ValidateTarget(Desired{Name: "@", Type: "CNAME"}, "route53"); err == nil {
		t.Fatal("apex CNAME on route53 must be rejected")
	}
	if err := ValidateTarget(Desired{Name: "@", Type: "CNAME"}, "cloudflare"); err != nil {
		t.Fatalf("cloudflare flattens an apex CNAME: %v", err)
	}
	if err := ValidateTarget(Desired{Name: "app", Type: "CNAME"}, "route53"); err != nil {
		t.Fatalf("subdomain CNAME is fine: %v", err)
	}
}

type fakeManager struct {
	zones map[string][]libdns.Record
}

func (f fakeManager) GetRecords(_ context.Context, zone string) ([]libdns.Record, error) {
	recs, ok := f.zones[zone]
	if !ok {
		return nil, ErrZoneNotFound
	}
	return recs, nil
}

func (f fakeManager) AppendRecords(context.Context, string, []libdns.Record) ([]libdns.Record, error) {
	return nil, nil
}

func (f fakeManager) SetRecords(context.Context, string, []libdns.Record) ([]libdns.Record, error) {
	return nil, nil
}

func (f fakeManager) DeleteRecords(context.Context, string, []libdns.Record) ([]libdns.Record, error) {
	return nil, nil
}

func TestManagerZoneFinder(t *testing.T) {
	t.Parallel()
	soa := rr("@", "SOA", "ns.example.com. hostmaster.example.com. 1 7200 900 1209600 86400")
	f := ManagerZoneFinder{Mgr: fakeManager{zones: map[string][]libdns.Record{
		"example.com.": {soa, rr("www", "A", "203.0.113.1")},
		// A provider pinned to one hosted zone answers for any name, but its
		// SOA is not at "@" under a wrong candidate.
		"app.example.com.": {rr("@", "A", "1.1.1.1")},
	}}}
	got, err := f.FindZone(context.Background(), "app.example.com")
	if err != nil || got != "example.com." {
		t.Fatalf("FindZone = %q %v", got, err)
	}
	if _, err := f.FindZone(context.Background(), "app.other.org"); err != ErrZoneNotFound {
		t.Fatalf("unknown zone error = %v", err)
	}
}

func TestCloudflareAPIFindZoneAndProxied(t *testing.T) {
	t.Parallel()
	var patched []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret-token" {
			http.Error(w, "no", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/zones" && r.URL.Query().Get("name") == "example.com":
			_, _ = w.Write([]byte(`{"success":true,"result":[{"id":"z1","name":"example.com"}]}`))
		case r.URL.Path == "/zones":
			_, _ = w.Write([]byte(`{"success":true,"result":[]}`))
		case r.URL.Path == "/zones/z1/dns_records" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"success":true,"result":[{"id":"r1","content":"203.0.113.5","proxied":false},{"id":"r2","content":"198.51.100.2","proxied":false}]}`))
		case strings.HasPrefix(r.URL.Path, "/zones/z1/dns_records/") && r.Method == http.MethodPatch:
			var body map[string]bool
			_ = json.NewDecoder(r.Body).Decode(&body)
			if !body["proxied"] {
				t.Errorf("patch body = %v", body)
			}
			patched = append(patched, strings.TrimPrefix(r.URL.Path, "/zones/z1/dns_records/"))
			_, _ = w.Write([]byte(`{"success":true,"result":{}}`))
		default:
			_, _ = w.Write([]byte(`{"success":false,"errors":[{"message":"unexpected"}]}`))
		}
	}))
	defer srv.Close()

	api := &CloudflareAPI{Token: "secret-token", BaseURL: srv.URL}
	zone, err := api.FindZone(context.Background(), "app.example.com")
	if err != nil || zone != "example.com." {
		t.Fatalf("FindZone = %q %v", zone, err)
	}
	if _, err := api.FindZone(context.Background(), "app.other.org"); err != ErrZoneNotFound {
		t.Fatalf("missing zone error = %v", err)
	}
	n, err := api.SetProxied(context.Background(), "example.com.", "app.example.com", "A", "203.0.113.5", true)
	if err != nil || n != 1 || len(patched) != 1 || patched[0] != "r1" {
		t.Fatalf("SetProxied = %d %v patched=%v", n, err, patched)
	}

	bad := &CloudflareAPI{Token: "wrong-token", BaseURL: srv.URL}
	if _, err := bad.FindZone(context.Background(), "app.example.com"); err == nil || strings.Contains(err.Error(), "wrong-token") {
		t.Fatalf("error must exist and never carry the token: %v", err)
	}
}
