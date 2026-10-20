package deploy

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/spec"
)

type fakeSecretScan struct {
	err  error
	root string
	env  map[string]string
	n    int
}

func (f *fakeSecretScan) BeforeDeploy(_ context.Context, _, _, root string, env map[string]string) error {
	f.n++
	f.root, f.env = root, env
	return f.err
}

func TestPlainEnvSkipsSecretsAndReferences(t *testing.T) {
	got := plainEnv(map[string]spec.EnvVar{
		"A": {Value: "literal"},
		"B": {Secret: true},
		"C": {From: "postgres.main.url"},
		"D": {Value: "x", Secret: true},
		"E": {Vault: &spec.VaultRef{}},
		"F": {},
	})
	if want := map[string]string{"A": "literal"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("plainEnv = %v, want %v", got, want)
	}
}

func TestPipeline_SecretScan_BlockStopsBeforeBuild(t *testing.T) {
	builder := &fakeBuilder{result: &build.Result{Tag: "levelrail/web:abc1234"}}
	svcStore := &fakeServiceStore{}
	blocked := errors.New("secret scan blocked this release: 1 suspected secret(s) found")
	hook := &fakeSecretScan{err: blocked}
	p := New(builder, svcStore, WithSecretScan(hook))

	_, err := p.Deploy(context.Background(), supplyChainRequest(), nil)
	if !errors.Is(err, blocked) {
		t.Fatalf("err = %v, want the block reason", err)
	}
	if hook.n != 1 || svcStore.saveCalls != 0 || builder.lastReq.Tag != "" {
		t.Fatalf("hook calls=%d saves=%d built=%q, want a stop before the build", hook.n, svcStore.saveCalls, builder.lastReq.Tag)
	}
}

func TestPipeline_SecretScan_PassesBuildContext(t *testing.T) {
	builder := &fakeBuilder{result: &build.Result{Tag: "levelrail/web:abc1234"}}
	hook := &fakeSecretScan{}
	p := New(builder, &fakeServiceStore{}, WithSecretScan(hook))
	req := supplyChainRequest()
	if _, err := p.Deploy(context.Background(), req, nil); err != nil {
		t.Fatal(err)
	}
	if hook.root != "/repo" {
		t.Fatalf("root = %q, want the build context", hook.root)
	}
}
