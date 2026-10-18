package application

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeBucketBindings struct{ bindings []store.StorageBucketBinding }

func (f *fakeBucketBindings) ListStorageBucketBindings(_ context.Context, app string) ([]store.StorageBucketBinding, error) {
	var out []store.StorageBucketBinding
	for _, b := range f.bindings {
		if b.AppName == app {
			out = append(out, b)
		}
	}
	return out, nil
}

func TestController_Reconcile_BucketBindings_InjectedUnderEnvPrefix(t *testing.T) {
	rt := newFakeRuntime(0)
	storageStore := &fakeStorageTargetStore{targets: map[string]store.BackupTarget{
		"bkt_1": {ID: "bkt_1", Name: "dest", Endpoint: "https://s3.example.com", Region: "auto", Bucket: "shared"},
	}}
	secretResolver := newFakeSecretResolver(map[string]string{
		"backup-target/bkt_1/access_key_id":     "AKIA_REAL",
		"backup-target/bkt_1/secret_access_key": "sk-real-secret",
	})
	bindings := &fakeBucketBindings{bindings: []store.StorageBucketBinding{
		{AppName: "web", EnvPrefix: "S3", Bucket: store.StorageBucket{ID: "sb_1", TargetID: "bkt_1", Prefix: "buckets/sb_1/"}},
		{AppName: "web", EnvPrefix: "MEDIA", Bucket: store.StorageBucket{ID: "sb_2", TargetID: "bkt_1", Prefix: "buckets/sb_2/"}},
		{AppName: "other", EnvPrefix: "S3", Bucket: store.StorageBucket{ID: "sb_3", TargetID: "bkt_1", Prefix: "buckets/sb_3/"}},
	}}
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80}
	c := New("web", &fakeStore{svc: desired}, rt, WithStorageTargets(storageStore), WithSecretResolver(secretResolver), WithBucketBindings(bindings))

	result, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if cond := conditionOf(t, result); cond.Status != reconcile.ConditionTrue {
		t.Errorf("condition = %+v, want Status=True", cond)
	}
	want := map[string]string{
		"S3_BUCKET": "shared", "S3_PREFIX": "buckets/sb_1/", "S3_ENDPOINT": "https://s3.example.com", "S3_REGION": "auto",
		"S3_ACCESS_KEY_ID": "AKIA_REAL", "S3_SECRET_ACCESS_KEY": "sk-real-secret",
		"MEDIA_BUCKET": "shared", "MEDIA_PREFIX": "buckets/sb_2/", "MEDIA_ACCESS_KEY_ID": "AKIA_REAL",
	}
	for k, wantV := range want {
		if gotV := rt.lastCreateEnv[k]; gotV != wantV {
			t.Errorf("container env %s = %q, want %q", k, gotV, wantV)
		}
	}
}
