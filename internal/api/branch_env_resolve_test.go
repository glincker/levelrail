package api

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestBranchMatchesPattern(t *testing.T) {
	tests := []struct {
		branch, pattern string
		want            bool
	}{
		{"main", "main", true},
		{"main", "master", false},
		{"release/1.0", "release/*", true},
		{"release/1.0/hotfix", "release/*", false},
		{"feature/foo", "feature/*", true},
		{"feature/foo", "release/*", false},
		{"anything", "*", true},
	}
	for _, tt := range tests {
		got, err := branchMatchesPattern(tt.branch, tt.pattern)
		if err != nil {
			t.Fatalf("branchMatchesPattern(%q, %q) error = %v", tt.branch, tt.pattern, err)
		}
		if got != tt.want {
			t.Errorf("branchMatchesPattern(%q, %q) = %v, want %v", tt.branch, tt.pattern, got, tt.want)
		}
	}
}

func TestBranchMatchesPattern_InvalidPattern(t *testing.T) {
	if _, err := branchMatchesPattern("main", "["); err == nil {
		t.Error("branchMatchesPattern with malformed pattern: want error, got nil")
	}
}

// fakeSecretResolver satisfies the narrow secretResolver surface
// applyBranchEnvOverrides needs, without a real internal/secrets.Manager.
type fakeSecretResolver struct {
	values map[string]string // "namespace/key" -> plaintext
	err    error
}

func (f *fakeSecretResolver) SetValueGuarded(context.Context, string, string, string, bool) error {
	return errors.New("not implemented")
}
func (f *fakeSecretResolver) ListKeys(context.Context, string) ([]store.SecretKeyInfo, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeSecretResolver) SetLocked(context.Context, string, string, bool) error {
	return errors.New("not implemented")
}
func (f *fakeSecretResolver) DeleteAll(context.Context, string) error {
	return errors.New("not implemented")
}
func (f *fakeSecretResolver) Exists(context.Context, string, string) (bool, error) {
	return false, errors.New("not implemented")
}
func (f *fakeSecretResolver) Resolve(_ context.Context, serviceName, envKey string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	v, ok := f.values[serviceName+"/"+envKey]
	if !ok {
		return "", errors.New("no value")
	}
	return v, nil
}

func TestApplyBranchEnvOverrides_NoMatch(t *testing.T) {
	svcSpec := &spec.Service{Env: map[string]spec.EnvVar{"KEY": {Value: "prod"}}}
	overrides := []store.ServiceBranchEnvOverride{
		{BranchPattern: "release/*", Key: "KEY", Value: "release-value"},
	}
	if err := applyBranchEnvOverrides(context.Background(), svcSpec, "web", "main", overrides, nil); err != nil {
		t.Fatalf("applyBranchEnvOverrides() error = %v", err)
	}
	if svcSpec.Env["KEY"].Value != "prod" {
		t.Errorf("Env[KEY] = %q, want %q (no matching branch override)", svcSpec.Env["KEY"].Value, "prod")
	}
}

func TestApplyBranchEnvOverrides_PlainMatchWinsOverAppLevel(t *testing.T) {
	svcSpec := &spec.Service{Env: map[string]spec.EnvVar{"KEY": {Value: "prod"}, "OTHER": {Value: "untouched"}}}
	overrides := []store.ServiceBranchEnvOverride{
		{BranchPattern: "release/*", Key: "KEY", Value: "release-value"},
	}
	if err := applyBranchEnvOverrides(context.Background(), svcSpec, "web", "release/1.0", overrides, nil); err != nil {
		t.Fatalf("applyBranchEnvOverrides() error = %v", err)
	}
	if svcSpec.Env["KEY"].Value != "release-value" {
		t.Errorf("Env[KEY] = %q, want %q", svcSpec.Env["KEY"].Value, "release-value")
	}
	if svcSpec.Env["OTHER"].Value != "untouched" {
		t.Errorf("Env[OTHER] = %q, want untouched", svcSpec.Env["OTHER"].Value)
	}
}

func TestApplyBranchEnvOverrides_ExactBeatsGlob(t *testing.T) {
	svcSpec := &spec.Service{Env: map[string]spec.EnvVar{}}
	overrides := []store.ServiceBranchEnvOverride{
		{BranchPattern: "release/*", Key: "KEY", Value: "glob-value"},
		{BranchPattern: "release/1.0", Key: "KEY", Value: "exact-value"},
	}
	if err := applyBranchEnvOverrides(context.Background(), svcSpec, "web", "release/1.0", overrides, nil); err != nil {
		t.Fatalf("applyBranchEnvOverrides() error = %v", err)
	}
	if svcSpec.Env["KEY"].Value != "exact-value" {
		t.Errorf("Env[KEY] = %q, want %q (exact match must win over glob)", svcSpec.Env["KEY"].Value, "exact-value")
	}
}

func TestApplyBranchEnvOverrides_Secret(t *testing.T) {
	svcSpec := &spec.Service{Env: map[string]spec.EnvVar{}}
	overrides := []store.ServiceBranchEnvOverride{
		{BranchPattern: "main", Key: "API_KEY", Secret: true},
	}
	resolver := &fakeSecretResolver{values: map[string]string{
		store.BranchEnvOverrideSecretsKey("web", "main") + "/API_KEY": "decrypted-value",
	}}
	if err := applyBranchEnvOverrides(context.Background(), svcSpec, "web", "main", overrides, resolver); err != nil {
		t.Fatalf("applyBranchEnvOverrides() error = %v", err)
	}
	if svcSpec.Env["API_KEY"].Value != "decrypted-value" {
		t.Errorf("Env[API_KEY] = %q, want the decrypted plaintext, never left secret-marked or blank", svcSpec.Env["API_KEY"].Value)
	}
}

func TestApplyBranchEnvOverrides_SecretNoResolverConfigured(t *testing.T) {
	svcSpec := &spec.Service{Env: map[string]spec.EnvVar{}}
	overrides := []store.ServiceBranchEnvOverride{
		{BranchPattern: "main", Key: "API_KEY", Secret: true},
	}
	if err := applyBranchEnvOverrides(context.Background(), svcSpec, "web", "main", overrides, nil); err == nil {
		t.Error("applyBranchEnvOverrides() with a secret override and no resolver: want error, got nil")
	}
}

func TestApplyBranchEnvOverrides_ResolveError(t *testing.T) {
	svcSpec := &spec.Service{Env: map[string]spec.EnvVar{}}
	overrides := []store.ServiceBranchEnvOverride{
		{BranchPattern: "main", Key: "API_KEY", Secret: true},
	}
	resolver := &fakeSecretResolver{err: errors.New("boom")}
	if err := applyBranchEnvOverrides(context.Background(), svcSpec, "web", "main", overrides, resolver); err == nil {
		t.Error("applyBranchEnvOverrides() with a failing resolver: want error, got nil")
	}
}

func TestApplyBranchEnvOverrides_EmptyBranch(t *testing.T) {
	svcSpec := &spec.Service{Env: map[string]spec.EnvVar{"KEY": {Value: "prod"}}}
	overrides := []store.ServiceBranchEnvOverride{
		{BranchPattern: "*", Key: "KEY", Value: "override"},
	}
	if err := applyBranchEnvOverrides(context.Background(), svcSpec, "web", "", overrides, nil); err != nil {
		t.Fatalf("applyBranchEnvOverrides() error = %v", err)
	}
	if svcSpec.Env["KEY"].Value != "prod" {
		t.Errorf("Env[KEY] = %q, want %q (an unknown branch never applies any override)", svcSpec.Env["KEY"].Value, "prod")
	}
}
