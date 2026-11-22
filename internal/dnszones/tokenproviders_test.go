package dnszones

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

func TestDigitalOceanUpsertDiffsRecords(t *testing.T) {
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("missing bearer token")
		}
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"domain_records":[{"id":1,"type":"A","name":"www","data":"1.1.1.1","ttl":300},{"id":2,"type":"A","name":"www","data":"2.2.2.2","ttl":300}]}`))
		case http.MethodPost:
			var b doRecord
			_ = json.NewDecoder(r.Body).Decode(&b)
			if b.Data != "3.3.3.3" || b.Name != "www" {
				t.Errorf("created %+v", b)
			}
		}
	}))
	defer srv.Close()
	d := NewDigitalOcean("tok")
	d.SetBaseURL(srv.URL, srv.Client())
	z := Zone{ID: "example.com", Name: "example.com"}
	err := d.UpsertRecordSet(context.Background(), z, RecordSet{Name: "www", Type: "A", TTL: 300, Values: []string{"1.1.1.1", "3.3.3.3"}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"GET /domains/example.com/records", "POST /domains/example.com/records", "DELETE /domains/example.com/records/2"}
	if !slices.Equal(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestHetznerTXTRoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Auth-API-Token") != "tok" {
			t.Errorf("missing token header")
		}
		_, _ = w.Write([]byte(`{"records":[{"id":"a","type":"TXT","name":"_acme-challenge","value":"\"abc def\"","ttl":60}],"meta":{"pagination":{"last_page":1}}}`))
	}))
	defer srv.Close()
	h := NewHetzner("tok")
	h.SetBaseURL(srv.URL, srv.Client())
	sets, err := h.ListRecordSets(context.Background(), Zone{ID: "z1", Name: "example.com"})
	if err != nil || len(sets) != 1 || sets[0].Values[0] != "abc def" {
		t.Fatalf("sets = %+v, err = %v", sets, err)
	}
}

func TestAzureListAndCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"value":[{"name":"www","type":"Microsoft.Network/dnszones/CNAME","properties":{"TTL":300,"CNAMERecord":{"cname":"app.example.net."}}},{"name":"@","type":"Microsoft.Network/dnszones/SOA","properties":{"TTL":3600}}]}`))
	}))
	defer srv.Close()
	a, err := NewAzure(`{"tenant_id":"t","client_id":"c","client_secret":"s","subscription_id":"sub","resource_group":"rg"}`)
	if err != nil {
		t.Fatal(err)
	}
	a.SetBaseURL(srv.URL, srv.Client(), a.cred)
	sets, err := a.ListRecordSets(context.Background(), Zone{ID: "example.com", Name: "example.com"})
	if err != nil || len(sets) != 1 || sets[0].Values[0] != "app.example.net" {
		t.Fatalf("sets = %+v, err = %v", sets, err)
	}
	if _, err := NewAzure(`{"tenant_id":"t"}`); err == nil {
		t.Fatal("incomplete credential accepted")
	}
}
