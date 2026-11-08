package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestAppSecretPutSendsValueAndListNeverReturnsOne(t *testing.T) {
	var got wireSecretValue
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/api/v1/apps/web/secrets/DB_URL" {
			_ = json.NewDecoder(r.Body).Decode(&got)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	r := &appSecretResource{resourceBase{client: NewClient(srv.URL, "tok")}}
	m := &appSecretModel{App: types.StringValue("web"), Key: types.StringValue("DB_URL"), Value: types.StringValue("s3cret-test")}
	if err := r.put(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	if got.Value != "s3cret-test" {
		t.Fatalf("value not sent: %+v", got)
	}
}

func TestAlertRuleIDAndWire(t *testing.T) {
	if got := ruleIDOf(types.StringValue("web/r1")); got != "r1" {
		t.Fatalf("id %q", got)
	}
	var m alertRuleModel
	m.fill("web", wireAlertRule{ID: "r1", Name: "cpu", Kind: "threshold", Threshold: 80, Enabled: true})
	w := m.wire()
	if w.Name != "cpu" || w.Threshold != 80 || !w.Enabled || m.ID.ValueString() != "web/r1" {
		t.Fatalf("wire %+v id %s", w, m.ID.ValueString())
	}
}
