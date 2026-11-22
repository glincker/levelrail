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

func TestPorkbunCredentialAndUpsert(t *testing.T) {
	for _, bad := range []string{"", "onlykey", `{"api_key":"k"}`} {
		if _, err := NewPorkbun(bad); err == nil {
			t.Fatalf("credential %q accepted", bad)
		}
	}
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		_ = json.NewDecoder(r.Body).Decode(&b)
		if b["apikey"] != "pk1" || b["secretapikey"] != "sk1" {
			t.Errorf("keys missing from body: %v", b)
		}
		calls = append(calls, r.URL.Path)
		switch r.URL.Path {
		case "/dns/retrieve/example.com":
			_, _ = w.Write([]byte(`{"status":"SUCCESS","records":[{"id":"7","name":"www.example.com","type":"A","content":"1.1.1.1","ttl":"600"},{"id":"8","name":"www.example.com","type":"A","content":"2.2.2.2","ttl":"600"}]}`))
		case "/dns/create/example.com":
			if b["name"] != "www" || b["content"] != "3.3.3.3" {
				t.Errorf("created %v", b)
			}
			_, _ = w.Write([]byte(`{"status":"SUCCESS"}`))
		default:
			_, _ = w.Write([]byte(`{"status":"SUCCESS"}`))
		}
	}))
	defer srv.Close()
	p, err := NewPorkbun("pk1:sk1")
	if err != nil {
		t.Fatal(err)
	}
	p.SetBaseURL(srv.URL, srv.Client())
	z := Zone{ID: "example.com", Name: "example.com"}
	if err := p.UpsertRecordSet(context.Background(), z, RecordSet{Name: "www", Type: "A", TTL: 600, Values: []string{"1.1.1.1", "3.3.3.3"}}); err != nil {
		t.Fatal(err)
	}
	want := []string{"/dns/retrieve/example.com", "/dns/create/example.com", "/dns/delete/example.com/8"}
	if !slices.Equal(calls, want) {
		t.Fatalf("calls = %v, want %v", calls, want)
	}
}

func TestPorkbunErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ERROR","message":"Invalid API key"}`))
	}))
	defer srv.Close()
	p, _ := NewPorkbun("a:b")
	p.SetBaseURL(srv.URL, srv.Client())
	if _, err := p.ListZones(context.Background()); err == nil {
		t.Fatal("ERROR status was not surfaced")
	}
}

func TestDeSECApexTXTRoundTrip(t *testing.T) {
	var gotPath string
	var gotBody desecRRSet
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Token tok" {
			t.Errorf("missing token header")
		}
		if r.Method == http.MethodPut {
			gotPath = r.URL.Path
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			return
		}
		_, _ = w.Write([]byte(`[{"subname":"","type":"SOA","ttl":3600,"records":["x"]},{"subname":"_acme-challenge","type":"TXT","ttl":3600,"records":["\"abc def\""]}]`))
	}))
	defer srv.Close()
	d := NewDeSEC("tok")
	d.SetBaseURL(srv.URL, srv.Client())
	z := Zone{ID: "example.com", Name: "example.com"}
	sets, err := d.ListRecordSets(context.Background(), z)
	if err != nil || len(sets) != 1 || sets[0].Values[0] != "abc def" {
		t.Fatalf("sets = %+v, err = %v", sets, err)
	}
	if err := d.UpsertRecordSet(context.Background(), z, RecordSet{Name: Apex, Type: "TXT", TTL: 3600, Values: []string{"hello world"}}); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/domains/example.com/rrsets/@/TXT/" || gotBody.SubName != "" || gotBody.Records[0] != `"hello world"` {
		t.Fatalf("put %s %+v", gotPath, gotBody)
	}
}

func TestGoDaddyReplacesOnlyOneSet(t *testing.T) {
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "sso-key k:s" {
			t.Errorf("missing sso-key header")
		}
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`[{"type":"MX","name":"@","data":"mail.example.com","ttl":600,"priority":10},{"type":"NS","name":"@","data":"ns1","ttl":600}]`))
		}
	}))
	defer srv.Close()
	g, err := NewGoDaddy("k:s")
	if err != nil {
		t.Fatal(err)
	}
	g.SetBaseURL(srv.URL, srv.Client())
	z := Zone{ID: "example.com", Name: "example.com"}
	sets, err := g.ListRecordSets(context.Background(), z)
	if err != nil || len(sets) != 1 || sets[0].Values[0] != "10 mail.example.com" {
		t.Fatalf("sets = %+v, err = %v", sets, err)
	}
	if err := g.UpsertRecordSet(context.Background(), z, RecordSet{Name: "www", Type: "A", TTL: 60, Values: []string{"1.2.3.4"}}); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(calls, "PUT /domains/example.com/records/A/www") {
		t.Fatalf("calls = %v", calls)
	}
	if _, err := NewGoDaddy("nosecret"); err == nil {
		t.Fatal("want credential error")
	}
}
