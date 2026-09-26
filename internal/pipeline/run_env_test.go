package pipeline

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestStepEnv_IncludesRunEnvWithoutInterpolating(t *testing.T) {
	jr := &jobRun{
		run:    store.PipelineRun{ID: "r1", AppName: "web"},
		def:    &Definition{},
		jd:     &Job{},
		row:    store.PipelineJob{Key: "test"},
		runEnv: map[string]string{"PREVIEW_PR_NUMBER": "42", "PREVIEW_BRANCH": "${{ secrets.X }}"},
	}

	env, err := jr.stepEnv(Step{}, Scope{})
	if err != nil {
		t.Fatalf("stepEnv() error = %v", err)
	}
	if env["PREVIEW_PR_NUMBER"] != "42" {
		t.Errorf("PREVIEW_PR_NUMBER = %q, want 42", env["PREVIEW_PR_NUMBER"])
	}
	if env["PREVIEW_BRANCH"] != "${{ secrets.X }}" {
		t.Errorf("PREVIEW_BRANCH = %q, want the literal branch name", env["PREVIEW_BRANCH"])
	}
	if env["PIPELINE_APP"] != "web" {
		t.Errorf("PIPELINE_APP = %q, want web", env["PIPELINE_APP"])
	}
}
