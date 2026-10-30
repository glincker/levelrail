package deploy

import (
	"context"
	"strings"
	"testing"
)

func TestBuildOnlyRejectsDockerfileOutsideSource(t *testing.T) {
	p := New(nil, nil)
	_, err := p.BuildOnly(context.Background(), BuildOnlyRequest{
		SourceDir:  t.TempDir(),
		Type:       "dockerfile",
		Dockerfile: "../../outside/Dockerfile",
		Tag:        "x:1",
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "dockerfile") {
		t.Fatalf("err = %v, want a dockerfile containment error", err)
	}
}
