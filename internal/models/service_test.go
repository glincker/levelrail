package models

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/gpu"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

type memSecrets struct {
	values  map[string]string
	failSet bool
}

func (m *memSecrets) SetValue(_ context.Context, ns, key, v string) error {
	if m.failSet {
		return errors.New("kms down")
	}
	m.values[ns+"/"+key] = v
	return nil
}
func (m *memSecrets) DeleteAll(context.Context, string) error { return nil }

func openDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func fallbackFn(host, label string) (string, bool) { return label + "." + host + ".sslip.io", true }

func newSvc(t *testing.T, secrets SecretWriter) (*Service, *store.DB) {
	t.Helper()
	db := openDB(t)
	return NewService(db, secrets, NewHostResolver("1-2-3-4", fallbackFn), "local-id"), db
}

func spec(name string) Spec {
	return Spec{Name: name, Engine: EngineOllama, ModelRef: "llama3.1:8b"}
}

func TestService_Create(t *testing.T) {
	ctx := context.Background()
	t.Run("stores hashed key and returns plaintext once", func(t *testing.T) {
		svc, db := newSvc(t, nil)
		got, err := svc.Create(ctx, CreateInput{Spec: spec("chat"), Domain: "Chat.Example.com"})
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if !strings.HasPrefix(got.APIKey, "lr-") || got.Model.Domain != "chat.example.com" || got.Model.GPUCount != -1 {
			t.Fatalf("Created = %+v", got)
		}
		stored, _ := db.GetModel(ctx, "chat")
		if stored.APIKeyHash == got.APIKey || !KeyMatches(got.APIKey, stored.APIKeyHash) || !strings.HasPrefix(got.APIKey, stored.APIKeyPrefix) {
			t.Errorf("stored hash/prefix wrong: %+v", stored)
		}
		v, _ := svc.Get(ctx, "chat")
		if v.BaseURL != "https://chat.example.com/v1" || v.Reason != "Pending" {
			t.Errorf("view = %+v", v)
		}
	})

	tests := []struct {
		name    string
		in      CreateInput
		secrets SecretWriter
		want    error
	}{
		{name: "bad name", in: CreateInput{Spec: spec("Bad_Name")}, want: ErrInvalid},
		{name: "bad engine", in: CreateInput{Spec: Spec{Name: "a", Engine: "tgi", ModelRef: "x/y"}}, want: ErrInvalid},
		{name: "bad ref for engine", in: CreateInput{Spec: Spec{Name: "a", Engine: EngineVLLM, ModelRef: "llama3:8b"}}, want: ErrInvalid},
		{name: "bad domain", in: CreateInput{Spec: spec("a"), Domain: "not a domain"}, want: ErrInvalid},
		{name: "hf token without secrets", in: CreateInput{Spec: spec("a"), HFToken: "hf_x"}, want: ErrSecretsUnavailable},
		{name: "unknown node", in: CreateInput{Spec: spec("a"), NodeID: "ghost"}, want: ErrNodeNotFound},
		{name: "quantization on ollama", in: CreateInput{Spec: Spec{Name: "a", Engine: EngineOllama, ModelRef: "llama3:8b", Quantization: "awq"}}, want: ErrInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _ := newSvc(t, tt.secrets)
			if _, err := svc.Create(ctx, tt.in); !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}

	t.Run("duplicate name", func(t *testing.T) {
		svc, _ := newSvc(t, nil)
		if _, err := svc.Create(ctx, CreateInput{Spec: spec("chat")}); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Create(ctx, CreateInput{Spec: spec("chat")}); !errors.Is(err, store.ErrModelExists) {
			t.Errorf("err = %v, want ErrModelExists", err)
		}
	})

	t.Run("node known to have no gpu is rejected", func(t *testing.T) {
		svc, db := newSvc(t, nil)
		_ = db.SetNodeGPU(ctx, store.LocalNodeGPUKey, gpu.Info{})
		if _, err := svc.Create(ctx, CreateInput{Spec: spec("chat")}); !errors.Is(err, ErrInvalid) {
			t.Errorf("err = %v, want ErrInvalid", err)
		}
	})

	t.Run("hf token goes to secrets, not the row", func(t *testing.T) {
		sec := &memSecrets{values: map[string]string{}}
		svc, _ := newSvc(t, sec)
		got, err := svc.Create(ctx, CreateInput{Spec: Spec{Name: "v", Engine: EngineVLLM, ModelRef: "org/model"}, HFToken: "hf_x"})
		if err != nil {
			t.Fatal(err)
		}
		if !got.Model.HFTokenSet || sec.values["model/v/HF_TOKEN"] != "hf_x" {
			t.Errorf("token flag/secret = %v/%v", got.Model.HFTokenSet, sec.values)
		}
	})

	t.Run("secret write failure rolls the row back", func(t *testing.T) {
		svc, db := newSvc(t, &memSecrets{values: map[string]string{}, failSet: true})
		if _, err := svc.Create(ctx, CreateInput{Spec: spec("chat"), HFToken: "hf_x"}); err == nil {
			t.Fatal("want error")
		}
		if _, err := db.GetModel(ctx, "chat"); !errors.Is(err, store.ErrModelNotFound) {
			t.Errorf("row must be rolled back, err = %v", err)
		}
	})
}

func TestService_LifecycleAndViews(t *testing.T) {
	ctx := context.Background()
	svc, db := newSvc(t, &memSecrets{values: map[string]string{}})
	created, err := svc.Create(ctx, CreateInput{Spec: spec("chat")})
	if err != nil {
		t.Fatal(err)
	}

	newKey, err := svc.RotateKey(ctx, "chat")
	if err != nil || newKey == created.APIKey {
		t.Fatalf("RotateKey = %q, %v", newKey, err)
	}
	stored, _ := db.GetModel(ctx, "chat")
	if KeyMatches(created.APIKey, stored.APIKeyHash) || !KeyMatches(newKey, stored.APIKeyHash) {
		t.Error("old key must stop matching and the new one match")
	}

	if err := svc.SetHFToken(ctx, "chat", "hf_new"); err != nil {
		t.Fatal(err)
	}
	stored, _ = db.GetModel(ctx, "chat")
	if !stored.HFTokenSet || stored.RestartNonce != 1 {
		t.Errorf("after SetHFToken = %+v", stored)
	}

	_ = db.UpsertConditions(ctx, ControllerName("chat"), []reconcile.Condition{{Type: "Ready", Status: reconcile.ConditionTrue, Reason: "ModelLoaded"}})
	list, err := svc.List(ctx)
	if err != nil || len(list) != 1 || !list[0].Ready || list[0].Reason != "ModelLoaded" {
		t.Fatalf("List = %+v, %v", list, err)
	}

	if err := svc.Restart(ctx, "chat"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, "chat"); err != nil {
		t.Fatal(err)
	}
	v, _ := svc.Get(ctx, "chat")
	if v.Reason != "Deleting" || v.Ready {
		t.Errorf("deleting view = %+v", v)
	}
	if _, err := svc.Get(ctx, "missing"); !errors.Is(err, store.ErrModelNotFound) {
		t.Errorf("Get missing err = %v", err)
	}
	if _, err := svc.RotateKey(ctx, "missing"); !errors.Is(err, store.ErrModelNotFound) {
		t.Errorf("RotateKey missing err = %v", err)
	}
	noSecrets, _ := newSvc(t, nil)
	if err := noSecrets.SetHFToken(ctx, "x", "t"); !errors.Is(err, ErrSecretsUnavailable) {
		t.Errorf("SetHFToken without secrets err = %v", err)
	}
}
