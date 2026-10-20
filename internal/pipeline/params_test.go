package pipeline

import (
	"errors"
	"strings"
	"testing"
)

const paramsYAML = `
version: 1
parameters:
  - { name: env, type: choice, choices: [staging, prod], default: staging }
  - { name: version, required: true, description: tag to ship }
  - { name: dry_run, type: boolean }
  - { name: note, default: "" }
on:
  manual: {}
  webhook: {}
jobs:
  a:
    steps: [{ uses: notify, with: { message: hi } }]
`

func TestResolveParametersMatrix(t *testing.T) {
	t.Parallel()
	def, issues := Validate([]byte(paramsYAML))
	if len(issues) != 0 {
		t.Fatalf("issues: %+v", issues)
	}
	tests := []struct {
		name    string
		given   map[string]string
		want    map[string]string
		wantErr string
	}{
		{"defaults filled", map[string]string{"version": "v1"}, map[string]string{"env": "staging", "version": "v1", "dry_run": "false", "note": ""}, ""},
		{"all set", map[string]string{"env": "prod", "version": "v2", "dry_run": "TRUE", "note": "x"}, map[string]string{"env": "prod", "version": "v2", "dry_run": "true", "note": "x"}, ""},
		{"missing required", map[string]string{}, nil, `input "version" is required`},
		{"empty required", map[string]string{"version": ""}, nil, `input "version" is required`},
		{"bad choice", map[string]string{"version": "v", "env": "dev"}, nil, "must be one of staging, prod"},
		{"bad boolean", map[string]string{"version": "v", "dry_run": "yes"}, nil, "must be true or false"},
		{"unknown name", map[string]string{"version": "v", "other": "1"}, nil, `unknown input "other"`},
		{"too long", map[string]string{"version": strings.Repeat("a", 33)}, nil, "longer than 32 bytes"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ResolveParameters(def, tc.given, 32)
			if tc.wantErr != "" {
				if !errors.Is(err, ErrInvalidInput) || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want ErrInvalidInput containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for k, v := range tc.want {
				if got[k] != v {
					t.Errorf("%s = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestParameterDeclarationValidation(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, params, want string }{
		{"bad type", "[{ name: a, type: number }]", "validation failed"},
		{"choice without choices", "[{ name: a, type: choice }]", "needs choices"},
		{"choices on string", "[{ name: a, type: string, choices: [x] }]", "only apply to a choice"},
		{"default not a choice", "[{ name: a, choices: [x, y], default: z }]", "must be one of x, y"},
		{"bad boolean default", "[{ name: a, type: boolean, default: maybe }]", "must be true or false"},
		{"bad name", "[{ name: 1a }]", "validation failed"},
		{"duplicate", "[{ name: a }, { name: a }]", "duplicate parameter"},
	}
	for _, tc := range tests {
		yaml := "version: 1\nparameters: " + tc.params + "\njobs:\n  a:\n    steps: [{ uses: notify, with: { message: hi } }]\n"
		_, issues := Validate([]byte(yaml))
		if len(issues) == 0 || !strings.Contains(issues[0].Message, tc.want) {
			t.Errorf("%s: issues = %+v, want %q", tc.name, issues, tc.want)
		}
	}
}

func TestLegacyManualInputsStillResolve(t *testing.T) {
	t.Parallel()
	def, issues := Validate([]byte("version: 1\non:\n  manual:\n    inputs:\n      env: { options: [a, b], default: a }\njobs:\n  a:\n    steps: [{ uses: notify, with: { message: hi } }]\n"))
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	got, err := ResolveParameters(def, map[string]string{"env": "b"}, 0)
	if err != nil || got["env"] != "b" {
		t.Fatalf("got %v, %v", got, err)
	}
	if _, err := ResolveParameters(def, map[string]string{"env": "c"}, 0); err == nil {
		t.Error("value outside options accepted")
	}
}

func TestWebhookRunRecordsPayloadAndExposesAsEnv(t *testing.T) {
	t.Parallel()
	h := newHarness(t, okHandler)
	yaml := "version: 1\non: { webhook: {} }\nparameters: [{ name: tag }]\njobs:\n  a:\n    image: x\n    steps: [{ run: echo hi }]\n"
	p := h.save(yaml)
	run := h.start(p, StartOptions{Trigger: TriggerWebhook, Actor: "webhook", Inputs: map[string]string{"tag": "v9"}, Payload: []byte(`{"x":"$(rm -rf /)"}`), PayloadContentType: "application/json"})
	if got := h.done(run.ID); got.Status != "succeeded" {
		t.Fatalf("run = %s %q", got.Status, got.Reason)
	}
	stored, _ := h.db.GetPipelineRun(t.Context(), run.ID)
	if stored.TriggerKind != TriggerWebhook || stored.TriggerPayload != `{"x":"$(rm -rf /)"}` || stored.TriggerContentType != "application/json" {
		t.Fatalf("stored run = %+v", stored)
	}
	script := h.rt.scripts[len(h.rt.scripts)-1]
	for _, want := range []string{"export PIPELINE_INPUT_TAG='v9'", "export PIPELINE_TRIGGER='webhook'", "export PIPELINE_WEBHOOK_PAYLOAD='{\"x\":\"$(rm -rf /)\"}'"} {
		if !strings.Contains(script, want) {
			t.Errorf("script missing %q:\n%s", want, script)
		}
	}
	if _, err := h.e.Start(t.Context(), h.save("version: 1\njobs:\n  a:\n    image: x\n    steps: [{ run: x }]\n"), StartOptions{Trigger: TriggerWebhook}); err == nil {
		t.Error("pipeline without on.webhook accepted a webhook trigger")
	}
}
