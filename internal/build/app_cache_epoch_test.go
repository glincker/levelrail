package build

import (
	"context"
	"testing"
)

type fakeEpochs map[string]string

func (f fakeEpochs) GetAppCacheEpoch(_ context.Context, name string) (string, error) {
	return f[name], nil
}

func (f fakeEpochs) EnsureAppCacheEpoch(_ context.Context, name string) (string, error) {
	if f[name] == "" {
		f[name] = "aaaa1111"
	}
	return f[name], nil
}

func TestAppCacheKeyChangesWithInstance(t *testing.T) {
	ctx := context.Background()
	e := fakeEpochs{}
	SetAppCacheEpochs(e)
	t.Cleanup(func() { SetAppCacheEpochs(nil) })

	if got := AppCacheKey(ctx, "web"); got != "web" {
		t.Fatalf("legacy key = %q", got)
	}
	first := EnsureAppCacheKey(ctx, "web")
	if first == "web" || first != AppCacheKey(ctx, "web") {
		t.Fatalf("instance key = %q", first)
	}
	delete(e, "web")
	e["web"] = "bbbb2222"
	if second := EnsureAppCacheKey(ctx, "web"); second == first {
		t.Fatalf("recreated app reused key %q", second)
	}
}
