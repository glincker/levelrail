package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestSettingWriteReadResetAndSensitive(t *testing.T) {
	ctx := context.Background()
	var calls []string
	var put string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch {
		case r.URL.Path == "/api/v1/settings/runtime/a.flag" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"key":"a.flag","kind":"bool","value":true,"source":"saved"}`))
		case r.URL.Path == "/api/v1/settings/runtime/a.secret":
			_, _ = w.Write([]byte(`{"key":"a.secret","kind":"string","value":"***","sensitive":true}`))
		case r.URL.Path == "/api/v1/settings/runtime/a.flag" && r.Method == http.MethodPut:
			var b map[string]json.RawMessage
			_ = json.NewDecoder(r.Body).Decode(&b)
			put = string(b["value"])
			_, _ = w.Write([]byte(`{}`))
		case r.Method == http.MethodDelete:
			_, _ = w.Write([]byte(`{}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	r := &settingResource{}
	r.client = NewClient(srv.URL, "tok")

	if err := r.write(ctx, settingModel{Key: strVal("a.flag"), Value: strVal("true")}); err != nil || put != "true" {
		t.Fatalf("bool must be sent as JSON: %q %v", put, err)
	}
	if err := r.write(ctx, settingModel{Key: strVal("a.secret"), Value: strVal("x")}); err == nil || !strings.Contains(err.Error(), "sensitive") {
		t.Fatalf("sensitive settings must be refused: %v", err)
	}
	if err := r.write(ctx, settingModel{Key: strVal("nope"), Value: strVal("x")}); err == nil {
		t.Fatal("an unknown key must fail")
	}
	if settingText(json.RawMessage(`"15m"`)) != "15m" || settingText(json.RawMessage(`[ "a" ]`)) != `["a"]` {
		t.Fatal("settingText")
	}

	calls = nil
	if err := r.client.do(ctx, http.MethodDelete, "/api/v1/settings/runtime/a.flag", nil, nil); err != nil || calls[0] != "DELETE /api/v1/settings/runtime/a.flag" {
		t.Fatalf("reset: %v %v", calls, err)
	}
}

func TestNodeLabelsKeepForeignAndRemoveOnlyOwn(t *testing.T) {
	ctx := context.Background()
	labels := map[string]string{"foreign": "1", "zone": "old"}
	taints := []wireTaint{{Key: "other", Effect: "NoSchedule"}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/nodes":
			_, _ = w.Write([]byte(`[{"id":"n1","name":"edge"}]`))
		case r.URL.Path == "/api/v1/nodes/n1/labels" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"labels": labels})
		case r.URL.Path == "/api/v1/nodes/n1/labels":
			var b struct{ Labels map[string]string }
			_ = json.NewDecoder(r.Body).Decode(&b)
			labels = b.Labels
		case r.URL.Path == "/api/v1/nodes/n1/taints" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"taints": taints})
		case r.URL.Path == "/api/v1/nodes/n1/taints":
			var b struct{ Taints []wireTaint }
			_ = json.NewDecoder(r.Body).Decode(&b)
			taints = b.Taints
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	r := &nodeLabelsResource{}
	r.client = NewClient(srv.URL, "tok")

	id, err := r.resolve(ctx, "edge")
	if err != nil || id != "n1" {
		t.Fatalf("resolve: %q %v", id, err)
	}
	if _, err := r.resolve(ctx, "ghost"); !isNotFound(err) {
		t.Fatalf("unknown node must be not found: %v", err)
	}
	want := nodeLabelsModel{
		Labels: types.MapValueMust(types.StringType, map[string]attr.Value{"zone": strVal("eu")}),
		Taints: []nodeTaintModel{{Key: strVal("gpu"), Value: strVal("a"), Effect: strVal("NoSchedule")}},
	}
	if err := r.apply(ctx, id, nodeLabelsModel{}, want); err != nil {
		t.Fatal(err)
	}
	if labels["zone"] != "eu" || labels["foreign"] != "1" || len(taints) != 2 {
		t.Fatalf("apply must merge: %v %v", labels, taints)
	}
	if err := r.apply(ctx, id, want, nodeLabelsModel{}); err != nil {
		t.Fatal(err)
	}
	if _, ok := labels["zone"]; ok || labels["foreign"] != "1" || len(taints) != 1 || taints[0].Key != "other" {
		t.Fatalf("delete must remove only its own: %v %v", labels, taints)
	}
}
