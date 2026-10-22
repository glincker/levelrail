package dnsrecords

import (
	"context"
	"testing"
	"time"

	"github.com/libdns/libdns"

	"github.com/GLINCKER/levelrail/internal/dnszones"
	"github.com/GLINCKER/levelrail/internal/dnszones/gcdtest"
)

func TestZoneManagerDNS01Flow(t *testing.T) {
	srv := gcdtest.New("proj-1", "example.com.")
	defer srv.Close()
	m := ZoneManager{P: &dnszones.GoogleCloud{BaseURL: srv.URL, ProjectID: "proj-1", HTTP: srv.Client()}}
	ctx := context.Background()

	zone, err := ManagerZoneFinder{Mgr: m}.FindZone(ctx, "app.example.com")
	if err != nil || zone != "example.com." {
		t.Fatalf("FindZone = %q, %v", zone, err)
	}

	challenge := func(v string) libdns.Record {
		return libdns.RR{Name: "_acme-challenge", Type: "TXT", TTL: time.Minute, Data: v}
	}
	if _, err := m.AppendRecords(ctx, zone, []libdns.Record{challenge("apex-token")}); err != nil {
		t.Fatalf("append 1: %v", err)
	}
	if _, err := m.AppendRecords(ctx, zone, []libdns.Record{challenge("wildcard-token")}); err != nil {
		t.Fatalf("append 2: %v", err)
	}
	var txt gcdtest.RRSet
	for _, s := range srv.Sets("example-com") {
		if s.Type == "TXT" {
			txt = s
		}
	}
	if len(txt.RRDatas) != 2 || txt.TTL != 60 || txt.Name != "_acme-challenge.example.com." {
		t.Fatalf("TXT set after appends = %+v", txt)
	}

	if _, err := m.DeleteRecords(ctx, zone, []libdns.Record{challenge("apex-token")}); err != nil {
		t.Fatalf("delete 1: %v", err)
	}
	if _, err := m.DeleteRecords(ctx, zone, []libdns.Record{challenge("wildcard-token")}); err != nil {
		t.Fatalf("delete 2: %v", err)
	}
	for _, s := range srv.Sets("example-com") {
		if s.Type == "TXT" {
			t.Fatalf("challenge TXT should be gone: %+v", s)
		}
	}

	if _, err := m.SetRecords(ctx, zone, []libdns.Record{libdns.RR{Name: "app", Type: "A", TTL: 5 * time.Minute, Data: "203.0.113.7"}}); err != nil {
		t.Fatalf("set: %v", err)
	}
	recs, err := m.GetRecords(ctx, zone)
	if err != nil {
		t.Fatalf("GetRecords: %v", err)
	}
	found := false
	for _, r := range recs {
		if rr := r.RR(); rr.Name == "app" && rr.Type == "A" && rr.Data == "203.0.113.7" && rr.TTL == 5*time.Minute {
			found = true
		}
	}
	if !found {
		t.Fatalf("A record missing from %v", recs)
	}

	if _, err := m.GetRecords(ctx, "other.org."); err == nil {
		t.Fatal("unknown zone should fail")
	}
}
