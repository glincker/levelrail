package pipeline

import (
	"os/exec"
	"strings"
	"testing"
)

func TestInterpolateScript_UntrustedValuesBecomeEnv(t *testing.T) {
	sc := Scope{Vars: map[string]string{
		"branch": "x; touch /tmp/pwned", "sha": "abc123", "matrix.go": "1.22", "secrets.T": "tok",
		"inputs.a-b": "$(id)", "needs.build.outputs.image": "img",
	}}
	tests := []struct {
		name, in, wantBody string
		wantEnv            map[string]string
	}{
		{"branch indirected", "echo ${{ branch }}", "echo ${PIPELINE_EXPR_BRANCH}", map[string]string{"PIPELINE_EXPR_BRANCH": "x; touch /tmp/pwned"}},
		{"literal vars spliced", "go ${{ matrix.go }} ${{sha}} ${{ secrets.T }}", "go 1.22 abc123 tok", map[string]string{}},
		{"repeat reuses one name", "${{ branch }}-${{ branch }}", "${PIPELINE_EXPR_BRANCH}-${PIPELINE_EXPR_BRANCH}", map[string]string{"PIPELINE_EXPR_BRANCH": "x; touch /tmp/pwned"}},
		{"punctuation sanitised", "${{ inputs.a-b }}", "${PIPELINE_EXPR_INPUTS_A_B}", map[string]string{"PIPELINE_EXPR_INPUTS_A_B": "$(id)"}},
		{"needs output indirected", "${{ needs.build.outputs.image }}", "${PIPELINE_EXPR_NEEDS_BUILD_OUTPUTS_IMAGE}", map[string]string{"PIPELINE_EXPR_NEEDS_BUILD_OUTPUTS_IMAGE": "img"}},
		{"unknown path is empty and indirected", "${{ nope }}", "${PIPELINE_EXPR_NOPE}", map[string]string{"PIPELINE_EXPR_NOPE": ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, env, err := InterpolateScript(tt.in, sc)
			if err != nil {
				t.Fatal(err)
			}
			if body != tt.wantBody {
				t.Errorf("body = %q, want %q", body, tt.wantBody)
			}
			if len(env) != len(tt.wantEnv) {
				t.Fatalf("env = %v, want %v", env, tt.wantEnv)
			}
			for k, v := range tt.wantEnv {
				if env[k] != v {
					t.Errorf("env[%s] = %q, want %q", k, env[k], v)
				}
			}
		})
	}
	if _, _, err := InterpolateScript("${{ secrets.NOPE }}", sc); err == nil {
		t.Error("missing secret must error")
	}
}

func TestInterpolateScript_HostileBranchDoesNotRunCode(t *testing.T) {
	hostile := "main'; echo INJECTED; echo '$(echo INJECTED)`echo INJECTED`"
	sc := Scope{Vars: map[string]string{"branch": hostile}}
	body, exprEnv, err := InterpolateScript(`echo "on ${{ branch }}"`, sc)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-s") //nolint:gosec // fixed argv, the script is the test input
	cmd.Stdin = strings.NewReader(strings.Replace(buildScript(exprEnv, body), "cd /workspace", "cd .", 1))
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("script failed: %v\n%s", err, got)
	}
	if strings.TrimSpace(string(got)) != "on "+hostile {
		t.Errorf("output = %q, want the hostile branch printed verbatim and never executed", got)
	}
}
