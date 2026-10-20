package deploy

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/build"
)

type fakeCacheProvider struct {
	s3      *build.S3Cache
	err     error
	records []string
}

func (f *fakeCacheProvider) S3Cache(context.Context, string) (*build.S3Cache, error) {
	return f.s3, f.err
}

func (f *fakeCacheProvider) RecordBuild(_ context.Context, _, warning string) {
	f.records = append(f.records, warning)
}

type warnBuilder struct {
	fakeBuilder
	warn string
}

func (w *warnBuilder) Build(ctx context.Context, req build.Request, progress func(build.ProgressEvent)) (*build.Result, error) {
	if w.warn != "" {
		build.EmitCacheWarning(progress, w.warn)
	}
	return w.fakeBuilder.Build(ctx, req, progress)
}

func TestPipeline_Deploy_BuildCache(t *testing.T) {
	s3 := &build.S3Cache{Bucket: "b", AccessKeyID: "k", SecretAccessKey: "s"}
	tests := []struct {
		name        string
		provider    *fakeCacheProvider
		builderWarn string
		wantS3      bool
		wantRecords []string
	}{
		{name: "no setting builds without cache and records nothing", provider: &fakeCacheProvider{}},
		{name: "setting passes s3 cache to the build", provider: &fakeCacheProvider{s3: s3}, wantS3: true, wantRecords: []string{""}},
		{name: "resolve failure fails open with a warning", provider: &fakeCacheProvider{err: errors.New("secret missing")}, wantRecords: []string{"build cache skipped: could not resolve the storage destination"}},
		{name: "build time cache warning is recorded", provider: &fakeCacheProvider{s3: s3}, builderWarn: "cache export failed", wantS3: true, wantRecords: []string{"cache export failed"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := &warnBuilder{fakeBuilder: fakeBuilder{result: &build.Result{Tag: "levelrail/web:abc1234"}}, warn: tt.builderWarn}
			p := New(builder, &fakeServiceStore{}, WithBuildCache(tt.provider))
			_, err := p.Deploy(context.Background(), Request{
				ServiceName: "web", Service: dockerfileService(), SourceDir: t.TempDir(), CommitSHA: "abc1234", ImageRepo: "levelrail/web",
			}, nil)
			if err != nil {
				t.Fatalf("Deploy: %v", err)
			}
			if got := builder.lastReq.S3Cache != nil; got != tt.wantS3 {
				t.Errorf("S3Cache passed = %v, want %v", got, tt.wantS3)
			}
			if len(tt.provider.records) != len(tt.wantRecords) {
				t.Fatalf("records = %v, want %v", tt.provider.records, tt.wantRecords)
			}
			for i, w := range tt.wantRecords {
				if tt.provider.records[i] != w {
					t.Errorf("record %d = %q, want %q", i, tt.provider.records[i], w)
				}
			}
		})
	}
}
