package ddns

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/dnszones"
)

type recordingZones struct {
	dnszones.Provider
	zone dnszones.Zone
	rs   dnszones.RecordSet
}

func (r *recordingZones) UpsertRecordSet(_ context.Context, z dnszones.Zone, rs dnszones.RecordSet) error {
	r.zone, r.rs = z, rs
	return nil
}

func TestZoneProviderWritesRecordInRegistrableZone(t *testing.T) {
	rec := &recordingZones{}
	p, err := newZoneProvider(rec, "home.example.co.uk", porkbunMinTTL)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Update(context.Background(), "203.0.113.9"); err != nil {
		t.Fatal(err)
	}
	if rec.zone.Name != "example.co.uk" || rec.rs.Name != "home" || rec.rs.Type != "A" || rec.rs.TTL != 600 || rec.rs.Values[0] != "203.0.113.9" {
		t.Fatalf("zone %+v set %+v", rec.zone, rec.rs)
	}
	if err := p.Update(context.Background(), "2001:db8::1"); err != nil || rec.rs.Type != "AAAA" {
		t.Fatalf("v6 update: %v, type %s", err, rec.rs.Type)
	}
}

func TestZoneProviderRejectsBadInput(t *testing.T) {
	if _, err := NewPorkbun("home.example.com", "onlykey"); err == nil {
		t.Fatal("porkbun credential without a secret accepted")
	}
	if _, err := NewDeSEC("", "tok"); err == nil {
		t.Fatal("empty hostname list accepted")
	}
	if _, err := NewDeSEC("com", "tok"); err == nil {
		t.Fatal("bare suffix accepted")
	}
	if s := (Settings{Provider: "bogus"}); s.validate() == nil {
		t.Fatal("unknown provider accepted")
	}
}

func TestSettingsAcceptGoDaddyPreset(t *testing.T) {
	s := DefaultSettings(func(string) string { return "" })
	s.Provider = ProviderGoDaddy
	if err := s.validate(); err != nil {
		t.Fatalf("godaddy provider rejected: %v", err)
	}
	if _, err := NewGoDaddy("home.example.com", "nosecret"); err == nil {
		t.Fatal("godaddy credential without a secret accepted")
	}
}
