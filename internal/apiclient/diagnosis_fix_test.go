package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func decodeApp(t *testing.T, s string) map[string]any {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader([]byte(s)))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestPatchAppMap(t *testing.T) {
	const appJSON = `{"name":"web","image":"i:1","port":3000,"env":{"A":"1"},"health":{"readiness":{"path":"/health","interval":5}},"resources":{"memory_bytes":268435456,"gpu":{"count":1}}}`
	tests := []struct {
		name    string
		changes []DiagnosisChange
		inputs  map[string]string
		wantErr error
		check   func(t *testing.T, app map[string]any)
	}{
		{"port", []DiagnosisChange{{Field: "port", From: "3000", To: "8080"}}, nil, nil, func(t *testing.T, a map[string]any) {
			if a["port"] != json.Number("8080") {
				t.Fatalf("port = %v", a["port"])
			}
		}},
		{"memory keeps gpu", []DiagnosisChange{{Field: "resources.memory_bytes", From: "268435456", To: "536870912"}}, nil, nil, func(t *testing.T, a map[string]any) {
			res := a["resources"].(map[string]any)
			if res["memory_bytes"] != json.Number("536870912") || res["gpu"] == nil {
				t.Fatalf("resources = %v", res)
			}
		}},
		{"health path", []DiagnosisChange{{Field: "health.readiness.path", From: "/health", To: "/healthz"}}, nil, nil, func(t *testing.T, a map[string]any) {
			p := a["health"].(map[string]any)["readiness"].(map[string]any)
			if p["path"] != "/healthz" || p["interval"] != json.Number("5") {
				t.Fatalf("probe = %v", p)
			}
		}},
		{"env input", []DiagnosisChange{{Field: "env.B", NeedsInput: true}}, map[string]string{"env.B": "x"}, nil, func(t *testing.T, a map[string]any) {
			if a["env"].(map[string]any)["B"] != "x" {
				t.Fatalf("env = %v", a["env"])
			}
		}},
		{"env missing input", []DiagnosisChange{{Field: "env.B", NeedsInput: true}}, nil, errors.New("value required"), nil},
		{"stale port", []DiagnosisChange{{Field: "port", From: "9999", To: "8080"}}, nil, ErrFixStale, nil},
		{"stale env set since diagnosis", []DiagnosisChange{{Field: "env.A", NeedsInput: true}}, map[string]string{"env.A": "x"}, ErrFixStale, nil},
		{"stale path", []DiagnosisChange{{Field: "health.readiness.path", From: "/other", To: "/x"}}, nil, ErrFixStale, nil},
		{"unsupported", []DiagnosisChange{{Field: "image", From: "", To: "x"}}, nil, errors.New("unsupported"), nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := decodeApp(t, appJSON)
			_, err := PatchAppMap(app, tc.changes, tc.inputs)
			if tc.wantErr != nil {
				if err == nil || (errors.Is(tc.wantErr, ErrFixStale) && !errors.Is(err, ErrFixStale)) {
					t.Fatalf("err = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			tc.check(t, app)
		})
	}
}

func TestApplyDiagnosisFixRoundTripsUnknownFields(t *testing.T) {
	var putBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"name":"web","image":"i:1","port":3000,"future_field":{"x":1}}`))
		case http.MethodPut:
			b, _ := io.ReadAll(r.Body)
			putBody = string(b)
			_, _ = w.Write(b)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "tok")
	fix := DiagnosisFix{Kind: "patch", Changes: []DiagnosisChange{{Field: "port", From: "3000", To: "8080"}}}
	if _, err := c.ApplyDiagnosisFix(context.Background(), "web", fix, nil, false); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(putBody), &got); err != nil {
		t.Fatal(err)
	}
	if got["port"] != float64(8080) || got["future_field"] == nil {
		t.Fatalf("put body = %s", putBody)
	}
	manual := DiagnosisFix{Kind: "manual", Hint: "chown it"}
	if _, err := c.ApplyDiagnosisFix(context.Background(), "web", manual, nil, false); err == nil {
		t.Fatal("manual fix must not apply")
	}
}
