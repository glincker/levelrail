package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestSaveAndGetDesiredService(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := DesiredService{
		Name:    "web",
		Image:   "levelrail/thesvg:abc1234",
		Port:    3000,
		Domains: []string{"app.example.com", "www.app.example.com"},
		Env:     map[string]string{"NODE_ENV": "production"},
		Resources: &ServiceResources{
			MemoryBytes: 512 * 1024 * 1024,
			NanoCPUs:    500_000_000,
		},
		Health: &ServiceHealth{
			Readiness: &ServiceProbe{Path: "/healthz", Interval: 5 * time.Second, Timeout: 2 * time.Second},
			Liveness:  &ServiceProbe{Path: "/healthz", Interval: 30 * time.Second, Failures: 3},
		},
	}

	if err := db.SaveDesiredService(ctx, want); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	got, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}

	if got.Name != want.Name || got.Image != want.Image || got.Port != want.Port {
		t.Errorf("scalar fields = %+v, want %+v", got, want)
	}
	if !reflect.DeepEqual(got.Domains, want.Domains) {
		t.Errorf("Domains = %+v, want %+v", got.Domains, want.Domains)
	}
	if !reflect.DeepEqual(got.Env, want.Env) {
		t.Errorf("Env = %+v, want %+v", got.Env, want.Env)
	}
	if got.Resources == nil || *got.Resources != *want.Resources {
		t.Errorf("Resources = %+v, want %+v", got.Resources, want.Resources)
	}
	if got.Health == nil || got.Health.Readiness == nil || *got.Health.Readiness != *want.Health.Readiness {
		t.Errorf("Health.Readiness = %+v, want %+v", got.Health, want.Health)
	}
	if got.Health.Liveness == nil || *got.Health.Liveness != *want.Health.Liveness {
		t.Errorf("Health.Liveness = %+v, want %+v", got.Health.Liveness, want.Health.Liveness)
	}
}

func TestSaveDesiredService_MinimalFieldsRoundTrip(t *testing.T) {
	// No Env, no Resources, no Health: the common case for a service
	// with nothing but an image and a port. Must round-trip cleanly,
	// not error on the way in or produce nil-pointer surprises on the
	// way out.
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{Name: "minimal", Image: "img:tag", Port: 8080}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	got, err := db.GetDesiredService(ctx, "minimal")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if got.Resources != nil {
		t.Errorf("Resources = %+v, want nil", got.Resources)
	}
	if got.Health != nil {
		t.Errorf("Health = %+v, want nil", got.Health)
	}
	if got.Env == nil {
		t.Error("Env = nil, want a non-nil (possibly empty) map")
	}
	if len(got.Env) != 0 {
		t.Errorf("Env = %+v, want empty", got.Env)
	}
	if got.Domains == nil {
		t.Error("Domains = nil, want a non-nil (possibly empty) slice")
	}
	if len(got.Domains) != 0 {
		t.Errorf("Domains = %+v, want empty", got.Domains)
	}
}

func TestSaveDesiredService_UpsertReplacesNotAccumulates(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 3000}); err != nil {
		t.Fatalf("first save: %v", err)
	}
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v2", Port: 4000}); err != nil {
		t.Fatalf("second save: %v", err)
	}

	got, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if got.Image != "img:v2" || got.Port != 4000 {
		t.Errorf("got %+v, want the second save's values, not a merge of both", got)
	}

	all, err := db.ListDesiredServices(ctx)
	if err != nil {
		t.Fatalf("ListDesiredServices() error = %v", err)
	}
	if len(all) != 1 {
		t.Errorf("expected exactly 1 row after two saves to the same name, got %d", len(all))
	}
}

func TestGetDesiredService_NotFound(t *testing.T) {
	db := openTestDB(t)
	_, err := db.GetDesiredService(context.Background(), "never-saved")
	if !errors.Is(err, ErrServiceNotFound) {
		t.Errorf("error = %v, want ErrServiceNotFound", err)
	}
}

func TestListDesiredServices_OrderedByName(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	for _, name := range []string{"worker", "api", "web"} {
		if err := db.SaveDesiredService(ctx, DesiredService{Name: name, Image: "img:tag", Port: 8080}); err != nil {
			t.Fatalf("save %q: %v", name, err)
		}
	}

	got, err := db.ListDesiredServices(ctx)
	if err != nil {
		t.Fatalf("ListDesiredServices() error = %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 services, got %d", len(got))
	}
	want := []string{"api", "web", "worker"}
	for i, name := range want {
		if got[i].Name != name {
			t.Errorf("position %d: got %q, want %q (expected alphabetical order)", i, got[i].Name, name)
		}
	}
}

func TestDeleteDesiredService(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:tag", Port: 8080}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := db.DeleteDesiredService(ctx, "web"); err != nil {
		t.Fatalf("DeleteDesiredService() error = %v", err)
	}

	if _, err := db.GetDesiredService(ctx, "web"); !errors.Is(err, ErrServiceNotFound) {
		t.Errorf("GetDesiredService after delete: error = %v, want ErrServiceNotFound", err)
	}
}

func TestDeleteDesiredService_NotFound(t *testing.T) {
	db := openTestDB(t)

	err := db.DeleteDesiredService(context.Background(), "never-saved")
	if !errors.Is(err, ErrServiceNotFound) {
		t.Errorf("error = %v, want ErrServiceNotFound", err)
	}
}

func TestDeleteDesiredService_DoesNotAffectOtherServices(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	for _, name := range []string{"api", "web"} {
		if err := db.SaveDesiredService(ctx, DesiredService{Name: name, Image: "img:tag", Port: 8080}); err != nil {
			t.Fatalf("seed %q: %v", name, err)
		}
	}

	if err := db.DeleteDesiredService(ctx, "web"); err != nil {
		t.Fatalf("DeleteDesiredService() error = %v", err)
	}

	if _, err := db.GetDesiredService(ctx, "api"); err != nil {
		t.Errorf("unrelated service api should be untouched, got error = %v", err)
	}
}

func TestSaveDesiredService_NewService_DefaultsToLocalNode(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	got, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if got.NodeID != "" {
		t.Errorf("NodeID = %q, want empty (local node) for a brand new service", got.NodeID)
	}
}

func TestUpdateServiceNode(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}
	if err := db.UpdateServiceNode(ctx, "web", "node-1"); err != nil {
		t.Fatalf("UpdateServiceNode() error = %v", err)
	}

	got, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if got.NodeID != "node-1" {
		t.Errorf("NodeID = %q, want node-1", got.NodeID)
	}
}

func TestUpdateServiceNode_NotFound(t *testing.T) {
	db := openTestDB(t)
	err := db.UpdateServiceNode(context.Background(), "nonexistent", "node-1")
	if !errors.Is(err, ErrServiceNotFound) {
		t.Errorf("UpdateServiceNode() error = %v, want ErrServiceNotFound", err)
	}
}

// TestSaveDesiredService_RedeployDoesNotResetNodeID is the real
// regression this task exists to prevent: internal/deploy.Pipeline
// calls SaveDesiredService on every ordinary redeploy with no opinion
// on placement at all. If that silently reset node_id back to local,
// every redeploy of a service an operator had explicitly moved to a
// remote node would move it back without anyone asking for that.
func TestSaveDesiredService_RedeployDoesNotResetNodeID(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("initial SaveDesiredService() error = %v", err)
	}
	if err := db.UpdateServiceNode(ctx, "web", "node-1"); err != nil {
		t.Fatalf("UpdateServiceNode() error = %v", err)
	}

	// A redeploy: a new image, same service, no NodeID opinion at all
	// (the zero value), exactly what internal/deploy.Pipeline sends.
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v2", Port: 8080}); err != nil {
		t.Fatalf("redeploy SaveDesiredService() error = %v", err)
	}

	got, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if got.Image != "img:v2" {
		t.Errorf("Image = %q, want img:v2 (the redeploy itself must still take effect)", got.Image)
	}
	if got.NodeID != "node-1" {
		t.Errorf("NodeID = %q, want node-1 (a redeploy must not silently un-assign a placed service)", got.NodeID)
	}
}

func TestSaveDesiredService_NewService_DefaultsToNoProject(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	got, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if got.ProjectID != "" {
		t.Errorf("ProjectID = %q, want empty (no project) for a brand new service", got.ProjectID)
	}
}

func TestUpdateServiceProject(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}
	if err := db.SaveProject(ctx, Project{ID: "proj_1", Name: "my-saas", CreatedAt: "2026-08-14T00:00:00Z"}); err != nil {
		t.Fatalf("SaveProject() error = %v", err)
	}
	if err := db.UpdateServiceProject(ctx, "web", "proj_1"); err != nil {
		t.Fatalf("UpdateServiceProject() error = %v", err)
	}

	got, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if got.ProjectID != "proj_1" {
		t.Errorf("ProjectID = %q, want proj_1", got.ProjectID)
	}
}

func TestUpdateServiceProject_BackToNoProject(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}
	if err := db.SaveProject(ctx, Project{ID: "proj_1", Name: "my-saas", CreatedAt: "2026-08-14T00:00:00Z"}); err != nil {
		t.Fatalf("SaveProject() error = %v", err)
	}
	if err := db.UpdateServiceProject(ctx, "web", "proj_1"); err != nil {
		t.Fatalf("UpdateServiceProject(proj_1) error = %v", err)
	}
	if err := db.UpdateServiceProject(ctx, "web", ""); err != nil {
		t.Fatalf("UpdateServiceProject(\"\") error = %v", err)
	}

	got, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if got.ProjectID != "" {
		t.Errorf("ProjectID = %q, want empty after moving back to no project", got.ProjectID)
	}
}

func TestUpdateServiceProject_NotFound(t *testing.T) {
	db := openTestDB(t)
	err := db.UpdateServiceProject(context.Background(), "nonexistent", "proj_1")
	if !errors.Is(err, ErrServiceNotFound) {
		t.Errorf("UpdateServiceProject() error = %v, want ErrServiceNotFound", err)
	}
}

// TestSaveDesiredService_RedeployDoesNotResetProjectID mirrors
// TestSaveDesiredService_RedeployDoesNotResetNodeID exactly, for the
// same reason: internal/deploy.Pipeline calls SaveDesiredService on
// every ordinary redeploy with no opinion on project membership at all,
// and that must never silently un-assign a service an operator had
// explicitly filed under a project.
func TestSaveDesiredService_RedeployDoesNotResetProjectID(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("initial SaveDesiredService() error = %v", err)
	}
	if err := db.SaveProject(ctx, Project{ID: "proj_1", Name: "my-saas", CreatedAt: "2026-08-14T00:00:00Z"}); err != nil {
		t.Fatalf("SaveProject() error = %v", err)
	}
	if err := db.UpdateServiceProject(ctx, "web", "proj_1"); err != nil {
		t.Fatalf("UpdateServiceProject() error = %v", err)
	}

	// A redeploy: a new image, same service, no ProjectID opinion at
	// all (the zero value), exactly what internal/deploy.Pipeline sends.
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v2", Port: 8080}); err != nil {
		t.Fatalf("redeploy SaveDesiredService() error = %v", err)
	}

	got, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if got.Image != "img:v2" {
		t.Errorf("Image = %q, want img:v2 (the redeploy itself must still take effect)", got.Image)
	}
	if got.ProjectID != "proj_1" {
		t.Errorf("ProjectID = %q, want proj_1 (a redeploy must not silently un-assign a project)", got.ProjectID)
	}
}

// TestListDesiredServicesByNode is TASKS.md 3.7's drain and
// delete-guard primitive: find what's placed on a node without
// listing every service and filtering client-side.
func TestListDesiredServicesByNode(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	for _, svc := range []DesiredService{
		{Name: "web", Image: "img:v1", Port: 8080},
		{Name: "worker", Image: "img:v1", Port: 8081},
		{Name: "api", Image: "img:v1", Port: 8082},
	} {
		if err := db.SaveDesiredService(ctx, svc); err != nil {
			t.Fatalf("SaveDesiredService(%s) error = %v", svc.Name, err)
		}
	}
	if err := db.UpdateServiceNode(ctx, "web", "node-1"); err != nil {
		t.Fatalf("UpdateServiceNode(web) error = %v", err)
	}
	if err := db.UpdateServiceNode(ctx, "worker", "node-1"); err != nil {
		t.Fatalf("UpdateServiceNode(worker) error = %v", err)
	}
	// api stays on the local node ("").

	got, err := db.ListDesiredServicesByNode(ctx, "node-1")
	if err != nil {
		t.Fatalf("ListDesiredServicesByNode(node-1) error = %v", err)
	}
	if len(got) != 2 || got[0].Name != "web" || got[1].Name != "worker" {
		t.Fatalf("ListDesiredServicesByNode(node-1) = %+v, want [web worker] ordered by name", got)
	}

	gotLocal, err := db.ListDesiredServicesByNode(ctx, "")
	if err != nil {
		t.Fatalf("ListDesiredServicesByNode(\"\") error = %v", err)
	}
	if len(gotLocal) != 1 || gotLocal[0].Name != "api" {
		t.Fatalf("ListDesiredServicesByNode(\"\") = %+v, want [api]", gotLocal)
	}

	gotEmpty, err := db.ListDesiredServicesByNode(ctx, "node-2")
	if err != nil {
		t.Fatalf("ListDesiredServicesByNode(node-2) error = %v", err)
	}
	if len(gotEmpty) != 0 {
		t.Errorf("ListDesiredServicesByNode(node-2) = %+v, want empty", gotEmpty)
	}
}

func TestSaveDesiredService_StrategyAndReplicas_RoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := DesiredService{Name: "web", Image: "img:v1", Port: 3000, Strategy: "recreate", Replicas: 3}
	if err := db.SaveDesiredService(ctx, want); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	got, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if got.Strategy != "recreate" {
		t.Errorf("Strategy = %q, want %q", got.Strategy, "recreate")
	}
	if got.Replicas != 3 {
		t.Errorf("Replicas = %d, want 3", got.Replicas)
	}
}

func TestSaveDesiredService_Labels_RoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := DesiredService{
		Name: "web", Image: "img:v1", Port: 3000,
		Labels: map[string]string{"team": "platform", "tier": "frontend"},
	}
	if err := db.SaveDesiredService(ctx, want); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	got, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if !reflect.DeepEqual(got.Labels, want.Labels) {
		t.Errorf("Labels = %+v, want %+v", got.Labels, want.Labels)
	}
}

func TestSaveDesiredService_NilLabels_RoundTripsToEmptyNonNilMap(t *testing.T) {
	// Mirrors nonNilMap's Env guarantee: a service saved with no labels
	// at all reads back a non-nil (if empty) map, one less nil-check for
	// every caller (internal/api's appResource, the frontend editor).
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 3000}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	got, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if got.Labels == nil {
		t.Fatal("Labels = nil, want a non-nil empty map")
	}
	if len(got.Labels) != 0 {
		t.Errorf("Labels = %+v, want empty", got.Labels)
	}
}

func TestSaveDesiredService_EmptyStrategyAndZeroReplicas_DefaultsPersisted(t *testing.T) {
	// A caller that never sets these two fields (every caller before
	// this migration existed, and internal/api's direct-image-registration
	// path today, which has no strategy/replicas concept yet) must
	// persist DefaultDeployStrategy/DefaultReplicas, not an empty string
	// or a zero replica count: DesiredService's own doc comment on these
	// fields requires them to always be the resolved value, and this is
	// the store layer's independent defense of that guarantee for any
	// caller, not just internal/deploy's.
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 3000}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}

	got, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if got.Strategy != DefaultDeployStrategy {
		t.Errorf("Strategy = %q, want default %q", got.Strategy, DefaultDeployStrategy)
	}
	if got.Replicas != DefaultReplicas {
		t.Errorf("Replicas = %d, want default %d", got.Replicas, DefaultReplicas)
	}
}

func TestRestartService_SetsNonNilNonce(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}
	before, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if before.RestartNonce != "" {
		t.Fatalf("RestartNonce before restart = %q, want empty", before.RestartNonce)
	}

	if err := db.RestartService(ctx, "web"); err != nil {
		t.Fatalf("RestartService() error = %v", err)
	}

	after, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if after.RestartNonce == "" {
		t.Error("RestartNonce after restart is empty, want a generated value")
	}
}

func TestRestartService_EachCallGeneratesADifferentNonce(t *testing.T) {
	// The reconciler treats a changed nonce as the signal to recreate a
	// container (internal/reconcile/application.ContainerName): two
	// restarts in a row must produce two different nonces, or the second
	// restart would be indistinguishable from a no-op.
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("SaveDesiredService() error = %v", err)
	}
	if err := db.RestartService(ctx, "web"); err != nil {
		t.Fatalf("first RestartService() error = %v", err)
	}
	first, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}

	if err := db.RestartService(ctx, "web"); err != nil {
		t.Fatalf("second RestartService() error = %v", err)
	}
	second, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}

	if first.RestartNonce == second.RestartNonce {
		t.Errorf("RestartNonce unchanged across two restarts: %q", first.RestartNonce)
	}
}

func TestRestartService_NotFound(t *testing.T) {
	db := openTestDB(t)
	err := db.RestartService(context.Background(), "nonexistent")
	if !errors.Is(err, ErrServiceNotFound) {
		t.Errorf("RestartService() error = %v, want ErrServiceNotFound", err)
	}
}

// TestSaveDesiredService_RedeployDoesNotResetRestartNonce mirrors
// TestSaveDesiredService_RedeployDoesNotResetNodeID exactly, for the
// same reason: internal/deploy.Pipeline calls SaveDesiredService on
// every ordinary redeploy with no opinion on RestartNonce at all
// (DesiredService{} zero value). If that silently reset restart_nonce,
// it would be indistinguishable from a real container recreation from
// the reconciler's point of view, which is harmless here (a redeploy
// already changes the image, which already forces a new container name
// on its own) but would still be the wrong general contract for a field
// that's deliberately excluded from full-record-replace semantics.
func TestSaveDesiredService_RedeployDoesNotResetRestartNonce(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 8080}); err != nil {
		t.Fatalf("initial SaveDesiredService() error = %v", err)
	}
	if err := db.RestartService(ctx, "web"); err != nil {
		t.Fatalf("RestartService() error = %v", err)
	}
	beforeRedeploy, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}

	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v2", Port: 8080}); err != nil {
		t.Fatalf("redeploy SaveDesiredService() error = %v", err)
	}

	got, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService() error = %v", err)
	}
	if got.RestartNonce != beforeRedeploy.RestartNonce {
		t.Errorf("RestartNonce = %q, want unchanged %q", got.RestartNonce, beforeRedeploy.RestartNonce)
	}
}
