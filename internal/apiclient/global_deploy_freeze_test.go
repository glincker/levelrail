package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_GlobalDeployFreeze(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		var body PutDeployFreezeRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		b, _ := json.Marshal(body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(DeployFreezeResource{Windows: body.Windows})
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "tok")

	if _, err := c.GetGlobalDeployFreeze(context.Background()); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if gotMethod != http.MethodGet || gotPath != "/api/v1/settings/deploy-freeze" {
		t.Errorf("got %s %s", gotMethod, gotPath)
	}

	if _, err := c.SetGlobalDeployFreeze(context.Background(), nil); err != nil {
		t.Fatalf("Set nil: %v", err)
	}
	if gotMethod != http.MethodPut || gotBody != `{"windows":[]}` {
		t.Errorf("nil windows: %s %s", gotMethod, gotBody)
	}

	out, err := c.SetGlobalDeployFreeze(context.Background(), []FreezeWindowResource{{Cron: "0 0 * * 5", Duration: "48h"}})
	if err != nil || len(out.Windows) != 1 {
		t.Errorf("Set windows: %v %+v", err, out)
	}
}
