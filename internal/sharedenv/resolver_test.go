package sharedenv

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeStore is a hand-written, in-memory fake for Store, matching the
// pattern internal/secrets' own fakeStore already establishes for its
// package tests.
type fakeStore struct {
	plain      map[string]map[string]string // scope key ("project:<id>" etc.) -> vars
	secretKeys map[string][]string          // scope key -> secret key names
	orgByProj  map[string]string
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		plain:      map[string]map[string]string{},
		secretKeys: map[string][]string{},
		orgByProj:  map[string]string{},
	}
}

func (f *fakeStore) ListProjectEnvVars(_ context.Context, projectID string) (map[string]string, error) {
	return f.plain["project:"+projectID], nil
}

func (f *fakeStore) ListProjectSecretEnvKeys(_ context.Context, projectID string) ([]string, error) {
	return f.secretKeys["project:"+projectID], nil
}

func (f *fakeStore) ListOrganizationEnvVarsForProject(_ context.Context, projectID string) (map[string]string, error) {
	orgID := f.orgByProj[projectID]
	if orgID == "" {
		return map[string]string{}, nil
	}
	return f.plain["org:"+orgID], nil
}

func (f *fakeStore) GetProjectOrganizationID(_ context.Context, projectID string) (string, error) {
	return f.orgByProj[projectID], nil
}

func (f *fakeStore) ListOrganizationSecretEnvKeys(_ context.Context, orgID string) ([]string, error) {
	return f.secretKeys["org:"+orgID], nil
}

func (f *fakeStore) ListEnvironmentEnvVars(_ context.Context, environmentID string) (map[string]string, error) {
	return f.plain["env:"+environmentID], nil
}

func (f *fakeStore) ListEnvironmentSecretEnvKeys(_ context.Context, environmentID string) ([]string, error) {
	return f.secretKeys["env:"+environmentID], nil
}

// fakeSecretResolver is a hand-written fake for SecretResolver: plain
// namespace/key -> plaintext lookup, no real crypto.
type fakeSecretResolver struct {
	values map[string]map[string]string // namespace -> key -> plaintext
	err    error
}

func (f *fakeSecretResolver) Resolve(_ context.Context, namespace, key string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	byKey, ok := f.values[namespace]
	if !ok {
		return "", errors.New("sharedenv test: no value")
	}
	v, ok := byKey[key]
	if !ok {
		return "", errors.New("sharedenv test: no value")
	}
	return v, nil
}

func TestResolver_ListProjectEnvVars(t *testing.T) {
	tests := []struct {
		name      string
		fs        *fakeStore
		secrets   SecretResolver
		projectID string
		want      map[string]string
		wantErr   bool
	}{
		{
			name: "no secrets manager configured returns plain vars only",
			fs: func() *fakeStore {
				fs := newFakeStore()
				fs.plain["project:p1"] = map[string]string{"A": "1"}
				fs.secretKeys["project:p1"] = []string{"B"}
				return fs
			}(),
			secrets:   nil,
			projectID: "p1",
			want:      map[string]string{"A": "1"},
		},
		{
			name: "secret keys are decrypted and merged with plain vars",
			fs: func() *fakeStore {
				fs := newFakeStore()
				fs.plain["project:p1"] = map[string]string{"A": "1"}
				fs.secretKeys["project:p1"] = []string{"B"}
				return fs
			}(),
			secrets: &fakeSecretResolver{values: map[string]map[string]string{
				store.ProjectEnvSecretsKey("p1"): {"B": "secret-value"},
			}},
			projectID: "p1",
			want:      map[string]string{"A": "1", "B": "secret-value"},
		},
		{
			name: "no secret keys set is a no-op even with a resolver configured",
			fs: func() *fakeStore {
				fs := newFakeStore()
				fs.plain["project:p1"] = map[string]string{"A": "1"}
				return fs
			}(),
			secrets:   &fakeSecretResolver{},
			projectID: "p1",
			want:      map[string]string{"A": "1"},
		},
		{
			name: "resolve failure propagates as an error",
			fs: func() *fakeStore {
				fs := newFakeStore()
				fs.secretKeys["project:p1"] = []string{"B"}
				return fs
			}(),
			secrets:   &fakeSecretResolver{err: errors.New("boom")},
			projectID: "p1",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Resolver{store: tt.fs, secrets: tt.secrets}
			got, err := r.ListProjectEnvVars(context.Background(), tt.projectID)
			if tt.wantErr {
				if err == nil {
					t.Fatal("ListProjectEnvVars() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ListProjectEnvVars() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ListProjectEnvVars() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolver_ListOrganizationEnvVarsForProject(t *testing.T) {
	fs := newFakeStore()
	fs.orgByProj["p1"] = "org1"
	fs.plain["org:org1"] = map[string]string{"BASE": "org-value"}
	fs.secretKeys["org:org1"] = []string{"ORG_SECRET"}
	secrets := &fakeSecretResolver{values: map[string]map[string]string{
		store.OrganizationEnvSecretsKey("org1"): {"ORG_SECRET": "shh"},
	}}

	r := &Resolver{store: fs, secrets: secrets}
	got, err := r.ListOrganizationEnvVarsForProject(context.Background(), "p1")
	if err != nil {
		t.Fatalf("ListOrganizationEnvVarsForProject() error = %v", err)
	}
	want := map[string]string{"BASE": "org-value", "ORG_SECRET": "shh"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListOrganizationEnvVarsForProject() = %v, want %v", got, want)
	}
}

func TestResolver_ListOrganizationEnvVarsForProject_NoOrganization(t *testing.T) {
	fs := newFakeStore()
	secrets := &fakeSecretResolver{}

	r := &Resolver{store: fs, secrets: secrets}
	got, err := r.ListOrganizationEnvVarsForProject(context.Background(), "p1")
	if err != nil {
		t.Fatalf("ListOrganizationEnvVarsForProject() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ListOrganizationEnvVarsForProject() = %v, want empty", got)
	}
}

func TestResolver_ListEnvironmentEnvVars(t *testing.T) {
	fs := newFakeStore()
	fs.plain["env:e1"] = map[string]string{"BASE": "env-value"}
	fs.secretKeys["env:e1"] = []string{"ENV_SECRET"}
	secrets := &fakeSecretResolver{values: map[string]map[string]string{
		store.EnvironmentEnvSecretsKey("e1"): {"ENV_SECRET": "shh"},
	}}

	r := &Resolver{store: fs, secrets: secrets}
	got, err := r.ListEnvironmentEnvVars(context.Background(), "e1")
	if err != nil {
		t.Fatalf("ListEnvironmentEnvVars() error = %v", err)
	}
	want := map[string]string{"BASE": "env-value", "ENV_SECRET": "shh"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListEnvironmentEnvVars() = %v, want %v", got, want)
	}
}

func TestNewResolver_NilSecretsManagerIsSafe(t *testing.T) {
	r := NewResolver(newFakeStore(), nil)
	if r.secrets != nil {
		t.Errorf("NewResolver(nil) left secrets non-nil, want nil so ListProjectEnvVars skips secret resolution")
	}
}
