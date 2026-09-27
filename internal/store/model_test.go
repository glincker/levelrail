package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/gpu"
)

func testModel(name, domain string) Model {
	return Model{Name: name, Engine: "ollama", ModelRef: "llama3.1:8b", GPUCount: -1, Domain: domain, APIKeyHash: "h", APIKeyPrefix: "lr-abcd"}
}

func TestModelCRUD(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	m := testModel("chat", "chat.example.com")
	m.GPUDeviceIDs = []string{"0", "1"}
	m.HFTokenSet = true
	if err := db.SaveModel(ctx, m); err != nil {
		t.Fatalf("SaveModel: %v", err)
	}
	got, err := db.GetModel(ctx, "chat")
	if err != nil {
		t.Fatalf("GetModel: %v", err)
	}
	if got.Engine != "ollama" || got.GPUCount != -1 || !got.HFTokenSet || !reflect.DeepEqual(got.GPUDeviceIDs, []string{"0", "1"}) {
		t.Errorf("GetModel = %+v", got)
	}

	if err := db.SaveModel(ctx, m); !errors.Is(err, ErrModelExists) {
		t.Errorf("duplicate name err = %v, want ErrModelExists", err)
	}
	if err := db.SaveModel(ctx, testModel("other", "chat.example.com")); !errors.Is(err, ErrModelDomainTaken) {
		t.Errorf("duplicate domain err = %v, want ErrModelDomainTaken", err)
	}
	if err := db.SaveModel(ctx, testModel("nodomain1", "")); err != nil {
		t.Fatalf("SaveModel nodomain1: %v", err)
	}
	if err := db.SaveModel(ctx, testModel("nodomain2", "")); err != nil {
		t.Errorf("two models without a domain must coexist: %v", err)
	}

	byDomain, err := db.GetModelByDomain(ctx, "chat.example.com")
	if err != nil || byDomain.Name != "chat" {
		t.Fatalf("GetModelByDomain = %+v, %v", byDomain, err)
	}
	if _, err := db.GetModelByDomain(ctx, ""); !errors.Is(err, ErrModelNotFound) {
		t.Errorf("empty domain lookup err = %v, want ErrModelNotFound", err)
	}

	if err := db.SetModelEndpoint(ctx, "chat", "127.0.0.1:1234"); err != nil {
		t.Fatalf("SetModelEndpoint: %v", err)
	}
	if err := db.RestartModel(ctx, "chat"); err != nil {
		t.Fatalf("RestartModel: %v", err)
	}
	if err := db.RotateModelAPIKey(ctx, "chat", "h2", "lr-zzzz"); err != nil {
		t.Fatalf("RotateModelAPIKey: %v", err)
	}
	got, _ = db.GetModel(ctx, "chat")
	if got.EndpointDial != "127.0.0.1:1234" || got.RestartNonce != 1 || got.APIKeyHash != "h2" || got.APIKeyPrefix != "lr-zzzz" {
		t.Errorf("after updates = %+v", got)
	}

	if err := db.MarkModelDeleting(ctx, "chat"); err != nil {
		t.Fatalf("MarkModelDeleting: %v", err)
	}
	got, _ = db.GetModel(ctx, "chat")
	if !got.Deleting || got.EndpointDial != "" {
		t.Errorf("after mark deleting = %+v", got)
	}
	if _, err := db.GetModelByDomain(ctx, "chat.example.com"); !errors.Is(err, ErrModelNotFound) {
		t.Errorf("deleting model must not resolve by domain, err = %v", err)
	}

	list, err := db.ListModels(ctx)
	if err != nil || len(list) != 3 {
		t.Fatalf("ListModels = %d, %v", len(list), err)
	}
	if err := db.DeleteModel(ctx, "chat"); err != nil {
		t.Fatalf("DeleteModel: %v", err)
	}
	if err := db.DeleteModel(ctx, "chat"); err != nil {
		t.Errorf("DeleteModel must be idempotent: %v", err)
	}
	if err := db.RestartModel(ctx, "chat"); !errors.Is(err, ErrModelNotFound) {
		t.Errorf("restart of missing model err = %v", err)
	}
}

func TestNodeGPURoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if _, ok, err := db.GetNodeGPU(ctx, "n1"); ok || err != nil {
		t.Fatalf("GetNodeGPU before report = ok %v err %v", ok, err)
	}
	info := gpu.Info{Present: true, DriverVersion: "550.1", RuntimeInstalled: true, Devices: []gpu.Device{{Index: 0, UUID: "GPU-a", Name: "A100", VRAMTotalMiB: 40960, VRAMUsedMiB: 10, UtilizationPercent: 3}}}
	if err := db.SetNodeGPU(ctx, "n1", info); err != nil {
		t.Fatalf("SetNodeGPU: %v", err)
	}
	info.Devices[0].VRAMUsedMiB = 99
	if err := db.SetNodeGPU(ctx, "n1", info); err != nil {
		t.Fatalf("SetNodeGPU upsert: %v", err)
	}
	got, ok, err := db.GetNodeGPU(ctx, "n1")
	if err != nil || !ok || !reflect.DeepEqual(got.Info, info) {
		t.Fatalf("GetNodeGPU = %+v ok=%v err=%v", got, ok, err)
	}
	all, err := db.ListNodeGPUs(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("ListNodeGPUs = %v, %v", all, err)
	}
}

func TestModelResidency(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveModel(ctx, testModel("chat", "")); err != nil {
		t.Fatal(err)
	}
	got, _ := db.GetModel(ctx, "chat")
	if got.Residency != ResidencyAlways || got.ResidencyState != ResidencyAwake || !got.LastActiveAt.IsZero() || got.IdleTTLSeconds != 0 {
		t.Fatalf("defaults = %+v", got)
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	if err := db.SetModelResidency(ctx, "chat", ResidencyOnDemand, 600, now); err != nil {
		t.Fatal(err)
	}
	got, _ = db.GetModel(ctx, "chat")
	if got.Residency != ResidencyOnDemand || got.IdleTTLSeconds != 600 || !got.LastActiveAt.Equal(now) {
		t.Fatalf("on demand = %+v", got)
	}

	if err := db.SetModelResidencyState(ctx, "chat", ResidencyAsleep); err != nil {
		t.Fatal(err)
	}
	later := now.Add(time.Minute)
	if err := db.TouchModel(ctx, "chat", later); err != nil {
		t.Fatal(err)
	}
	got, _ = db.GetModel(ctx, "chat")
	if got.ResidencyState != ResidencyAsleep || !got.LastActiveAt.Equal(later) {
		t.Fatalf("after touch = %+v", got)
	}

	if err := db.SleepModel(ctx, "chat"); err != nil {
		t.Fatal(err)
	}
	got, _ = db.GetModel(ctx, "chat")
	if !got.LastActiveAt.IsZero() {
		t.Fatalf("sleep must clear activity: %+v", got)
	}

	if err := db.SetModelResidency(ctx, "chat", ResidencyAlways, 0, now); err != nil {
		t.Fatal(err)
	}
	got, _ = db.GetModel(ctx, "chat")
	if got.Residency != ResidencyAlways || got.ResidencyState != ResidencyAwake {
		t.Fatalf("always must clear the asleep state: %+v", got)
	}

	for name, fn := range map[string]func() error{
		"touch":  func() error { return db.TouchModel(ctx, "nope", now) },
		"state":  func() error { return db.SetModelResidencyState(ctx, "nope", ResidencyAwake) },
		"sleep":  func() error { return db.SleepModel(ctx, "nope") },
		"config": func() error { return db.SetModelResidency(ctx, "nope", ResidencyAlways, 0, now) },
	} {
		if err := fn(); !errors.Is(err, ErrModelNotFound) {
			t.Errorf("%s on a missing model = %v, want ErrModelNotFound", name, err)
		}
	}
}
