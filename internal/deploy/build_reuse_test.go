package deploy

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/build"
)

func TestPipeline_Deploy_Dockerfile_ReusesUnchangedBuild(t *testing.T) {
	builder := &fakeBuilder{result: &build.Result{Tag: "levelrail/web:abc1234"}}
	p := New(builder, &fakeServiceStore{}, WithImageInspector(fakeInspector{id: "sha256:img"}))
	req := Request{ServiceName: "web", Service: dockerfileService(), SourceDir: "/repo", CommitSHA: "abc1234", ImageRepo: "levelrail/web"}

	for i := 0; i < 2; i++ {
		if _, err := p.Deploy(context.Background(), req, nil); err != nil {
			t.Fatalf("Deploy() #%d error = %v", i, err)
		}
	}
	if builder.calls != 1 {
		t.Fatalf("builds = %d, want 1: the second deploy should reuse the image", builder.calls)
	}

	req.Service.Build.Args = map[string]string{"FLAG": "on"}
	if _, err := p.Deploy(context.Background(), req, nil); err != nil {
		t.Fatalf("Deploy() with changed args error = %v", err)
	}
	if builder.calls != 2 {
		t.Fatalf("builds = %d, want 2 after the build settings changed", builder.calls)
	}

	req.NoCache = true
	if _, err := p.Deploy(context.Background(), req, nil); err != nil {
		t.Fatalf("Deploy() no-cache error = %v", err)
	}
	if builder.calls != 3 {
		t.Fatalf("builds = %d, want 3: no-cache always rebuilds", builder.calls)
	}
}
