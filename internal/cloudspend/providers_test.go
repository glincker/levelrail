package cloudspend

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/ocisign"
)

func fixedCfg() func() Config {
	return func() Config {
		return Config{MaxPages: 3, Now: func() time.Time { return time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC) }}
	}
}

func TestOracleSpendForecastBudgets(t *testing.T) {
	var usageReqs []ociUsageRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Signature ") {
			http.Error(w, "unsigned", http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, oracleUsagePath):
			var req ociUsageRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			usageReqs = append(usageReqs, req)
			if r.URL.Query().Get("page") == "" && req.Forecast == nil {
				w.Header().Set("opc-next-page", "p2")
				_, _ = w.Write([]byte(`{"items":[{"service":"Compute","computedAmount":4,"currency":"USD","extra":true}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"items":[{"service":"Block Storage","computedAmount":1,"currency":"USD"},{"service":"x","currency":"USD"}]}`))
		case strings.HasPrefix(r.URL.Path, oracleBudgetPath):
			_, _ = w.Write([]byte(`[{"id":"b1","displayName":"cap","amount":50,"resetPeriod":"MONTHLY","actualSpend":5,"forecastedSpend":20,"timeSpendComputed":"2026-11-10T22:00:00Z"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	r := NewOracleReader(ocisign.Signer{KeyID: "t/u/f", Key: key, AllowInsecure: true}, "ocid1.tenancy.oc1..x", srv.URL, srv.URL, fixedCfg())
	rep, err := r.Spend(context.Background(), SpendQuery{AccountID: "a"})
	if err != nil || rep.Totals[0].Amount != 5 || len(rep.ByService) != 2 || rep.DataAsOf == nil {
		t.Fatalf("rep %+v err %v", rep, err)
	}
	if usageReqs[0].Granularity != "DAILY" || !strings.HasSuffix(usageReqs[0].TimeStarted, "T00:00:00Z") {
		t.Fatalf("request %+v", usageReqs[0])
	}
	fc, err := r.Forecast(context.Background(), SpendQuery{AccountID: "a"})
	if err != nil || fc.Source != ForecastProvider || fc.Amount != 1 {
		t.Fatalf("fc %+v err %v", fc, err)
	}
	b, err := r.Budgets(context.Background(), SpendQuery{})
	if err != nil || len(b) != 1 || b[0].ForecastedSpend != 20 || b[0].DataAsOf == nil {
		t.Fatalf("budgets %+v err %v", b, err)
	}
}

func TestOracleErrorKeepsOnlyStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"code":"NotAuthorized","message":"ocid1.tenancy.oc1..secret"}`))
	}))
	defer srv.Close()
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	r := NewOracleReader(ocisign.Signer{KeyID: "t/u/f", Key: key, AllowInsecure: true}, "t", srv.URL, srv.URL, fixedCfg())
	_, err := r.Spend(context.Background(), SpendQuery{})
	if err == nil || strings.Contains(err.Error(), "ocid1") || ErrorCode(err) != "auth" {
		t.Fatalf("err %v code %s", err, ErrorCode(err))
	}
}

func TestDigitalOceanBalance(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" || r.URL.Path != "/customers/my/balance" {
			http.Error(w, "bad", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"month_to_date_usage":"10.00","generated_at":"2026-11-10T20:00:00Z"}`))
	}))
	defer srv.Close()
	r := NewDigitalOceanReader("tok", srv.URL, fixedCfg())
	rep, err := r.Spend(context.Background(), SpendQuery{AccountID: "a"})
	if err != nil || rep.Totals[0] != (Total{Currency: "USD", Amount: 10}) || rep.Source != SourceBalance || rep.Estimated {
		t.Fatalf("rep %+v err %v", rep, err)
	}
	fc, err := r.Forecast(context.Background(), SpendQuery{AccountID: "a"})
	if err != nil || fc.Source != ForecastLinear || fc.Amount < 29 || fc.Amount > 31 {
		t.Fatalf("fc %+v err %v", fc, err)
	}
}

func TestHetznerEstimate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/server_types":
			_, _ = w.Write([]byte(`{"server_types":[{"name":"cx22","prices":[{"location":"fsn1","price_hourly":{"gross":"0.01"},"price_monthly":{"gross":"2.00"}}]}],"meta":{"pagination":{"next_page":null}}}`))
		case "/servers":
			_, _ = w.Write([]byte(`{"servers":[
			 {"id":7,"name":"old","created":"2026-10-01T00:00:00Z","server_type":{"name":"cx22"},"location":{"name":"fsn1"}},
			 {"id":8,"name":"new","created":"2026-11-10T00:00:00Z","server_type":{"name":"cx22"},"datacenter":{"location":{"name":"fsn1"}}},
			 {"id":9,"name":"unpriced","created":"2026-10-01T00:00:00Z","server_type":{"name":"zz"},"location":{"name":"fsn1"}}],
			 "meta":{"pagination":{"next_page":null}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	rep, err := NewHetznerReader("tok", srv.URL, fixedCfg()).Spend(context.Background(), SpendQuery{AccountID: "a", ByResource: true})
	if err != nil || !rep.Estimated || rep.Source != SourceEstimate {
		t.Fatalf("rep %+v err %v", rep, err)
	}
	// old is capped at 2.00, new ran 24h at 0.01.
	if got := rep.Totals[0]; got.Currency != "EUR" || got.Amount < 2.23 || got.Amount > 2.25 || len(rep.ByResource) != 2 {
		t.Fatalf("totals %+v resources %+v", rep.Totals, rep.ByResource)
	}
}

func TestRegistry(t *testing.T) {
	if _, err := New("nope", nil, fixedCfg()); err == nil {
		t.Fatal("unknown provider must fail")
	}
	if got := Providers(); len(got) != 4 {
		t.Fatalf("providers %v", got)
	}
	if _, err := New(ProviderHetzner, Credentials{"token": "t"}, fixedCfg()); err != nil {
		t.Fatal(err)
	}
}
