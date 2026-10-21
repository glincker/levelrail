package dnszones

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/dnszones/gcdtest"
)

func newGCDClient(srv *gcdtest.Server) *GoogleCloud {
	return &GoogleCloud{BaseURL: srv.URL, ProjectID: "proj-1", HTTP: srv.Client()}
}

func TestGoogleCloudRecordCRUD(t *testing.T) {
	srv := gcdtest.New("proj-1", "example.com.")
	defer srv.Close()
	g := newGCDClient(srv)
	ctx := context.Background()

	zones, err := g.ListZones(ctx)
	if err != nil || len(zones) != 1 {
		t.Fatalf("ListZones = %v, %v", zones, err)
	}
	z := zones[0]
	if z.Name != "example.com" || z.Provider != ProviderGoogleCloud || len(z.NameServers) != 1 {
		t.Fatalf("zone = %+v", z)
	}

	for _, rs := range []RecordSet{
		{Name: "app", Type: "A", TTL: 300, Values: []string{"203.0.113.10"}},
		{Name: "app", Type: "AAAA", TTL: 300, Values: []string{"2001:db8::1"}},
		{Name: "www", Type: "CNAME", TTL: 300, Values: []string{"app.example.com."}},
		{Name: "_acme-challenge", Type: "TXT", TTL: 60, Values: []string{"token value"}},
	} {
		if err := g.UpsertRecordSet(ctx, z, rs); err != nil {
			t.Fatalf("upsert %s %s: %v", rs.Name, rs.Type, err)
		}
	}
	if err := g.UpsertRecordSet(ctx, z, RecordSet{Name: "app", Type: "A", TTL: 120, Values: []string{"203.0.113.11", "203.0.113.12"}}); err != nil {
		t.Fatalf("replace A: %v", err)
	}

	var rawTXT string
	for _, s := range srv.Sets("example-com") {
		if s.Type == "TXT" {
			rawTXT = s.RRDatas[0]
		}
	}
	if rawTXT != `"token value"` {
		t.Fatalf("TXT stored as %q, want quoted", rawTXT)
	}

	sets, err := g.ListRecordSets(ctx, z)
	if err != nil {
		t.Fatalf("ListRecordSets: %v", err)
	}
	byKey := map[string]RecordSet{}
	for _, s := range sets {
		byKey[s.Name+"/"+s.Type] = s
	}
	if a := byKey["app/A"]; a.TTL != 120 || !slices.Equal(a.Values, []string{"203.0.113.11", "203.0.113.12"}) {
		t.Fatalf("A = %+v", a)
	}
	if txt := byKey["_acme-challenge/TXT"]; !slices.Equal(txt.Values, []string{"token value"}) {
		t.Fatalf("TXT = %+v", txt)
	}
	if _, ok := byKey["@/SOA"]; !ok {
		t.Fatalf("apex SOA missing: %v", byKey)
	}

	if err := g.DeleteRecordSet(ctx, z, Key{Name: "www", Type: "CNAME"}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := g.DeleteRecordSet(ctx, z, Key{Name: "www", Type: "CNAME"}); err != nil {
		t.Fatalf("deleting a missing set should be a no-op: %v", err)
	}
	for _, s := range srv.Sets("example-com") {
		if s.Name == "www.example.com." {
			t.Fatalf("www still present: %+v", s)
		}
	}
}

func TestGoogleCloudZoneLifecycleAndErrors(t *testing.T) {
	srv := gcdtest.New("proj-1")
	defer srv.Close()
	g := newGCDClient(srv)
	ctx := context.Background()

	z, err := g.CreateZone(ctx, "Shop.Example.org")
	if err != nil || z.Name != "shop.example.org" || z.ID != "zone-shop-example-org" {
		t.Fatalf("CreateZone = %+v, %v", z, err)
	}
	if got, err := g.GetZone(ctx, z.ID); err != nil || got.ID != z.ID {
		t.Fatalf("GetZone = %+v, %v", got, err)
	}
	if err := g.DeleteZone(ctx, z.ID); err != nil {
		t.Fatalf("DeleteZone: %v", err)
	}
	if _, err := g.GetZone(ctx, z.ID); !errors.Is(err, ErrZoneNotFound) {
		t.Fatalf("GetZone after delete = %v, want ErrZoneNotFound", err)
	}

	bad := &GoogleCloud{BaseURL: srv.URL, ProjectID: "other", HTTP: srv.Client()}
	_, err = bad.ListZones(ctx)
	var ge *GoogleCloudError
	if !errors.As(err, &ge) || ge.Status != 404 {
		t.Fatalf("wrong project error = %v", err)
	}
}

func TestNewGoogleCloudRejectsBadCredential(t *testing.T) {
	for _, in := range []string{"not json", `{}`, `{"project_id":"p"}`} {
		_, err := NewGoogleCloud(in)
		if err == nil {
			t.Fatalf("NewGoogleCloud(%q) succeeded", in)
		}
		if strings.Contains(err.Error(), in) && in != "" && len(in) > 8 {
			t.Fatalf("error leaks credential text: %v", err)
		}
	}
}
