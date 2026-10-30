package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestSSAMergeOwnershipAndConflicts(t *testing.T) {
	current, _ := flattenSpec(map[string]any{"service": map[string]any{"port": 80, "replicas": 2, "domains": []any{"a.example.com"}}})
	owners := []store.ManagedField{{Path: "/spec/service/port", Manager: "ci"}, {Path: "/spec/service/replicas", Manager: "ops"}}

	same := ssaMerge{current: current, sent: map[string]any{"/spec/service/port": 80}, owners: owners, manager: "ops"}.run()
	if len(same.conflicts) != 0 || len(same.owners) != 2 {
		t.Fatalf("equal value should co-own: %+v", same)
	}

	clash := ssaMerge{current: current, sent: map[string]any{"/spec/service/port": 8080}, owners: owners, manager: "ops"}.run()
	if clash.merged != nil || len(clash.conflicts) != 1 || clash.conflicts[0].Manager != "ci" {
		t.Fatalf("expected one conflict with ci: %+v", clash)
	}

	forced := ssaMerge{current: current, sent: map[string]any{"/spec/service/port": 8080}, owners: owners, manager: "ops", force: true}.run()
	if forced.merged["/spec/service/port"] != 8080 {
		t.Fatalf("force did not set the field: %+v", forced.merged)
	}
	for _, o := range forced.owners {
		if o.Manager == "ci" {
			t.Fatalf("force left ci owning %s", o.Path)
		}
	}
	// ops stopped sending replicas, so the field it alone owned goes away.
	if _, ok := forced.merged["/spec/service/replicas"]; ok {
		t.Fatalf("dropped field kept: %+v", forced.merged)
	}

	removed := ssaMerge{current: current, sent: map[string]any{"/spec/service/domains": ssaNull{}}, owners: nil, manager: "edit"}.run()
	if _, ok := removed.merged["/spec/service/domains"]; ok {
		t.Fatalf("null did not remove the field: %+v", removed.merged)
	}
	spec := unflattenSpec(forced.merged)
	if svc, _ := spec["service"].(map[string]any); svc["port"] != 8080 {
		t.Fatalf("unflatten = %+v", spec)
	}
}

func TestV2ApplyRefusesBadRequestsAndUnsupportedKinds(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, target, ctype, body string
		want                      int
	}{
		{"no manager", "/api/v2/apps/web", "application/apply-patch+yaml", "spec: {service: {port: 81}}", http.StatusBadRequest},
		{"merge patch", "/api/v2/apps/web?fieldManager=ci", "application/merge-patch+json", `{"spec":{}}`, http.StatusUnsupportedMediaType},
		{"name mismatch", "/api/v2/apps/web?fieldManager=ci", "application/apply-patch+yaml", "metadata: {name: api}\nspec: {service: {port: 81}}", http.StatusBadRequest},
		{"stale version", "/api/v2/apps/web?fieldManager=ci&resourceVersion=999999", "application/apply-patch+yaml", "spec: {service: {port: 81}}", http.StatusConflict},
		{"nodes", "/api/v2/nodes/n1?fieldManager=ci", "application/apply-patch+yaml", "spec: {labels: {a: b}}", http.StatusMethodNotAllowed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := authedRequest(t, cookie, http.MethodPatch, c.target, c.body)
			req.Header.Set("Content-Type", c.ctype)
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, req)
			if rec.Code != c.want {
				t.Fatalf("status = %d, want %d: %s", rec.Code, c.want, rec.Body.String())
			}
		})
	}
}

func TestV2ApplyDryRunMergesWithoutWriting(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	req := authedRequest(t, cookie, http.MethodPatch, "/api/v2/apps/web?fieldManager=ci&dryRun=All", `{"kind":"App","spec":{"service":{"port":8080}}}`)
	req.Header.Set("Content-Type", "application/apply-patch+json")
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("dry run: %d %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		DryRun   bool `json:"dryRun"`
		Metadata struct {
			ManagedFields []v2ManagedField `json:"managedFields"`
		} `json:"metadata"`
		Spec map[string]any `json:"spec"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.DryRun || len(resp.Metadata.ManagedFields) != 1 || resp.Metadata.ManagedFields[0].Manager != "ci" {
		t.Fatalf("dry run response = %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"port":8080`) || !strings.Contains(rec.Body.String(), "nginx:1") {
		t.Fatalf("merged spec lost a field: %s", rec.Body.String())
	}
	if owners, _ := db.ListManagedFields(ctx, store.ResourceKindApp, "web"); len(owners) != 0 {
		t.Fatalf("dry run recorded ownership: %+v", owners)
	}
	if svc, _ := db.GetDesiredService(ctx, "web"); svc.Port != 80 {
		t.Fatalf("dry run changed the app: port %d", svc.Port)
	}
}
