package models

import (
	"reflect"
	"strings"
	"testing"
)

func TestSpecValidate(t *testing.T) {
	tests := []struct {
		name    string
		spec    Spec
		wantErr string
	}{
		{name: "ollama ok", spec: Spec{Name: "chat", Engine: EngineOllama, ModelRef: "llama3.1:8b"}},
		{name: "ollama namespaced ok", spec: Spec{Name: "chat", Engine: EngineOllama, ModelRef: "library/llama3.1:8b-instruct-q4_K_M"}},
		{name: "vllm ok", spec: Spec{Name: "v", Engine: EngineVLLM, ModelRef: "meta-llama/Llama-3.1-8B-Instruct", Quantization: "awq", ContextLength: 8192}},
		{name: "llamacpp with quant suffix ok", spec: Spec{Name: "l", Engine: EngineLlamaCpp, ModelRef: "bartowski/Llama-3.2-3B-Instruct-GGUF:Q4_K_M"}},
		{name: "name uppercase", spec: Spec{Name: "Chat", Engine: EngineOllama, ModelRef: "a"}, wantErr: "name"},
		{name: "name too long", spec: Spec{Name: strings.Repeat("a", 43), Engine: EngineOllama, ModelRef: "a"}, wantErr: "name"},
		{name: "unknown engine", spec: Spec{Name: "a", Engine: "tgi", ModelRef: "a"}, wantErr: "not supported"},
		{name: "vllm needs repo", spec: Spec{Name: "a", Engine: EngineVLLM, ModelRef: "llama3"}, wantErr: "HuggingFace"},
		{name: "ref with shell chars", spec: Spec{Name: "a", Engine: EngineVLLM, ModelRef: "org/model; rm -rf /"}, wantErr: "not valid"},
		{name: "ollama quantization", spec: Spec{Name: "a", Engine: EngineOllama, ModelRef: "a", Quantization: "q4"}, wantErr: "tag"},
		{name: "llamacpp quantization field", spec: Spec{Name: "a", Engine: EngineLlamaCpp, ModelRef: "o/m", Quantization: "q4"}, wantErr: ":quant"},
		{name: "bad quantization", spec: Spec{Name: "a", Engine: EngineVLLM, ModelRef: "o/m", Quantization: "a b"}, wantErr: "quantization"},
		{name: "negative gpu", spec: Spec{Name: "a", Engine: EngineOllama, ModelRef: "a", GPUCount: -2}, wantErr: "gpu count"},
		{name: "huge context", spec: Spec{Name: "a", Engine: EngineOllama, ModelRef: "a", ContextLength: 1 << 30}, wantErr: "context"},
		{name: "empty device id", spec: Spec{Name: "a", Engine: EngineOllama, ModelRef: "a", GPUDeviceIDs: []string{""}}, wantErr: "device"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.spec.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestCommandAndEnv(t *testing.T) {
	v := Spec{Engine: EngineVLLM, ModelRef: "org/m", ContextLength: 4096, Quantization: "awq"}
	want := []string{"--model", "org/m", "--served-model-name", "org/m", "--host", "0.0.0.0", "--port", "8000", "--max-model-len", "4096", "--quantization", "awq", "--tensor-parallel-size", "2"}
	if got := Command(v, 2); !reflect.DeepEqual(got, want) {
		t.Errorf("vllm command = %v, want %v", got, want)
	}
	if got := Command(v, 1); strings.Contains(strings.Join(got, " "), "tensor-parallel") {
		t.Errorf("single gpu must not set tensor parallelism: %v", got)
	}

	l := Spec{Engine: EngineLlamaCpp, ModelRef: "o/m-GGUF:Q4_K_M", ContextLength: 2048}
	wantL := []string{"-hf", "o/m-GGUF:Q4_K_M", "--alias", "o/m-GGUF:Q4_K_M", "--host", "0.0.0.0", "--port", "8080", "-ngl", "999", "-c", "2048"}
	if got := Command(l, 1); !reflect.DeepEqual(got, wantL) {
		t.Errorf("llamacpp command = %v, want %v", got, wantL)
	}
	if got := Command(Spec{Engine: EngineOllama}, 1); got != nil {
		t.Errorf("ollama uses the image entrypoint, command = %v", got)
	}

	env := Env(Spec{Engine: EngineOllama, ContextLength: 8192}, "ignored")
	if env["OLLAMA_CONTEXT_LENGTH"] != "8192" || env["OLLAMA_KEEP_ALIVE"] != "-1" || env["HF_TOKEN"] != "" {
		t.Errorf("ollama env = %v", env)
	}
	env = Env(Spec{Engine: EngineVLLM}, "tok")
	if env["HF_TOKEN"] != "tok" || env["HF_HOME"] == "" {
		t.Errorf("vllm env = %v", env)
	}
	if ShmBytes(EngineVLLM) == 0 || ShmBytes(EngineOllama) != 0 {
		t.Error("only vllm needs a larger /dev/shm")
	}
}

func TestEngineNamesAndFor(t *testing.T) {
	if got := EngineNames(); !reflect.DeepEqual(got, []string{"llamacpp", "ollama", "vllm"}) {
		t.Errorf("EngineNames = %v", got)
	}
	if _, ok := EngineFor("nope"); ok {
		t.Error("EngineFor(nope) = ok")
	}
}

func TestAPIKey(t *testing.T) {
	k1, h1, p1, err := NewAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	k2, _, _, _ := NewAPIKey()
	if k1 == k2 || !strings.HasPrefix(k1, "lr-") || !strings.HasPrefix(k1, p1) || len(p1) != 7 {
		t.Errorf("key/prefix = %q/%q", k1, p1)
	}
	if !KeyMatches(k1, h1) || KeyMatches(k2, h1) || KeyMatches("", h1) {
		t.Error("KeyMatches wrong")
	}
	if strings.Contains(h1, k1) {
		t.Error("hash must not contain the key")
	}
}
