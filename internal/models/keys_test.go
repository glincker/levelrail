package models

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func svcWithModel(t *testing.T) (*Service, *store.DB, Created) {
	t.Helper()
	svc, db := newSvc(t, nil)
	created, err := svc.Create(context.Background(), CreateInput{Spec: spec("chat")})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return svc, db, created
}

func TestService_DefaultKeyListed(t *testing.T) {
	svc, _, created := svcWithModel(t)
	keys, err := svc.ListKeys(context.Background(), "chat")
	if err != nil || len(keys) != 1 {
		t.Fatalf("ListKeys = %+v, %v", keys, err)
	}
	if keys[0].Name != "default" || keys[0].Status != KeyActive || len(keys[0].KeyPrefix) != 8 || created.APIKey[:8] != keys[0].KeyPrefix {
		t.Errorf("default key = %+v", keys[0])
	}
}

func TestService_CreateKeyValidation(t *testing.T) {
	svc, _, _ := svcWithModel(t)
	ctx := context.Background()
	past := time.Now().Add(-time.Hour)
	tests := []struct {
		name string
		in   CreateKeyInput
		want error
	}{
		{"bad name", CreateKeyInput{Name: "no spaces"}, ErrInvalid},
		{"empty name", CreateKeyInput{}, ErrInvalid},
		{"past expiry", CreateKeyInput{Name: "a", ExpiresAt: &past}, ErrInvalid},
		{"negative rpm", CreateKeyInput{Name: "a", Limits: KeyLimits{RPM: -1}}, ErrInvalid},
		{"engine admin path", CreateKeyInput{Name: "a", Limits: KeyLimits{AllowPaths: []string{"/api/pull"}}}, ErrInvalid},
		{"duplicate default", CreateKeyInput{Name: "default"}, ErrKeyExists},
		{"ok", CreateKeyInput{Name: "ci", Limits: KeyLimits{RPM: 10, TPM: 1000, MaxParallel: 2, AllowPaths: []string{"/v1/chat/completions"}}}, nil},
	}
	for _, tt := range tests {
		got, err := svc.CreateKey(ctx, "chat", tt.in)
		if !errors.Is(err, tt.want) {
			t.Errorf("%s: err = %v, want %v", tt.name, err, tt.want)
		}
		if err == nil && (got.Plaintext == "" || got.KeyHash == "") {
			t.Errorf("%s: missing plaintext or hash", tt.name)
		}
	}
	if _, err := svc.CreateKey(ctx, "nope", CreateKeyInput{Name: "a"}); !errors.Is(err, store.ErrModelNotFound) {
		t.Errorf("unknown model err = %v", err)
	}
}

func TestService_CreateKeyCap(t *testing.T) {
	t.Setenv(envMaxKeys, "2")
	svc, _, _ := svcWithModel(t)
	ctx := context.Background()
	if _, err := svc.CreateKey(ctx, "chat", CreateKeyInput{Name: "a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateKey(ctx, "chat", CreateKeyInput{Name: "b"}); !errors.Is(err, ErrInvalid) {
		t.Errorf("over the cap err = %v", err)
	}
}

func TestService_RotateWithGraceAndRevoke(t *testing.T) {
	svc, db, _ := svcWithModel(t)
	ctx := context.Background()
	changed := 0
	svc.SetKeyChangeHook(func() { changed++ })
	created, err := svc.CreateKey(ctx, "chat", CreateKeyInput{Name: "ci", Limits: KeyLimits{RPM: 5, AllowModels: []string{"llama"}}})
	if err != nil {
		t.Fatal(err)
	}
	grace := 30 * time.Minute
	rotated, err := svc.RotateKeyByID(ctx, "chat", created.ID, &grace)
	if err != nil {
		t.Fatalf("RotateKeyByID: %v", err)
	}
	if rotated.Name != "ci" || rotated.RPM != 5 || len(rotated.AllowModels) != 1 || rotated.Plaintext == created.Plaintext {
		t.Errorf("rotated = %+v", rotated)
	}
	keys, _ := svc.ListKeys(ctx, "chat")
	states := map[string]KeyStatus{}
	for _, k := range keys {
		states[k.ID] = k.Status
	}
	if states[created.ID] != KeyRotating || states[rotated.ID] != KeyActive {
		t.Errorf("states = %v", states)
	}
	if _, err := svc.RotateKeyByID(ctx, "chat", created.ID, nil); !errors.Is(err, ErrInvalid) {
		t.Errorf("rotating a rotating key err = %v", err)
	}
	bad := -time.Second
	if _, err := svc.RotateKeyByID(ctx, "chat", rotated.ID, &bad); !errors.Is(err, ErrInvalid) {
		t.Errorf("negative grace err = %v", err)
	}
	zero := time.Duration(0)
	if _, err := svc.RotateKeyByID(ctx, "chat", rotated.ID, &zero); err != nil {
		t.Fatalf("zero grace rotate: %v", err)
	}
	if err := svc.RevokeKey(ctx, "chat", created.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.RevokeKey(ctx, "chat", "missing"); !errors.Is(err, ErrKeyNotFound) {
		t.Errorf("revoke missing err = %v", err)
	}
	if changed < 4 {
		t.Errorf("key change hook fired %d times", changed)
	}
	old, _ := db.GetModelKey(ctx, "chat", rotated.ID)
	if old.RevokedAt == nil {
		t.Error("zero-grace rotation did not revoke the old key")
	}
}

func TestService_RotateKeyStillMeansDefault(t *testing.T) {
	svc, _, created := svcWithModel(t)
	ctx := context.Background()
	newKey, err := svc.RotateKey(ctx, "chat")
	if err != nil {
		t.Fatal(err)
	}
	keys, _ := svc.ListKeys(ctx, "chat")
	if len(keys) != 1 || !KeyMatches(newKey, keys[0].KeyHash) || KeyMatches(created.APIKey, keys[0].KeyHash) {
		t.Errorf("legacy rotate did not replace the default key: %+v", keys)
	}
}

func TestService_Usage(t *testing.T) {
	svc, db, _ := svcWithModel(t)
	ctx := context.Background()
	hour := time.Now().UTC().Truncate(time.Hour)
	def := store.DefaultModelKeyID("chat")
	rows := []store.ModelUsage{
		{ModelName: "chat", KeyID: def, HourStart: hour.Add(-time.Hour), Requests: 3, Status2xx: 2, Status5xx: 1, InputTokens: 30, OutputTokens: 60, UsageRequests: 2, DurationMsSum: 300, TTFTMsSum: 90, TTFTCount: 3},
		{ModelName: "chat", KeyID: def, HourStart: hour, Requests: 1, Status2xx: 1, DurationMsSum: 100},
		{ModelName: "chat", KeyID: "gone", HourStart: hour, Requests: 2, Status4xx: 2, RateLimited: 2},
	}
	if err := db.AddModelUsage(ctx, rows); err != nil {
		t.Fatal(err)
	}
	rep, err := svc.Usage(ctx, "chat", 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Totals.Requests != 6 || rep.Totals.InputTokens != 30 || rep.Totals.RateLimited != 2 || rep.Totals.AvgTTFTMs != 30 || rep.Totals.AvgDurationMs != 66 {
		t.Errorf("totals = %+v", rep.Totals)
	}
	if len(rep.Series) != 2 || rep.Series[0].Requests != 3 || rep.Series[1].Requests != 3 {
		t.Errorf("series = %+v", rep.Series)
	}
	if len(rep.Keys) != 1 || rep.Keys[0].Requests != 4 || rep.Keys[0].Name != "default" {
		t.Errorf("keys = %+v", rep.Keys)
	}
	if rep.Note == "" {
		t.Error("usage note missing")
	}
	if _, err := svc.Usage(ctx, "chat", 0); !errors.Is(err, ErrInvalid) {
		t.Errorf("zero window err = %v", err)
	}
}
