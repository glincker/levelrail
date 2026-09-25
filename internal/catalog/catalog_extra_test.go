package catalog

import (
	"strings"
	"testing"
)

func TestTemplates_ImagesAreTagged(t *testing.T) {
	for _, tpl := range Templates {
		f := templateByID(t, tpl.ID)
		for name, svc := range f.Services {
			img := svc.Image
			slash := strings.LastIndex(img, "/")
			if !strings.Contains(img[slash+1:], ":") {
				t.Errorf("template %q service %q: image %q has no tag", tpl.ID, name, img)
			}
		}
	}
}

func TestTemplates_RequiresGPU(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{"vllm", true},
		{"text-generation-inference", true},
		{"comfyui", true},
		{"stable-diffusion-webui", true},
		{"jupyter-gpu", true},
		{"llama-cpp", false},
		{"text-embeddings-inference", false},
		{"ollama", false},
	}
	byID := make(map[string]Template, len(Templates))
	for _, tpl := range Templates {
		byID[tpl.ID] = tpl
	}
	for _, tt := range tests {
		tpl, ok := byID[tt.id]
		if !ok {
			t.Errorf("no template %q", tt.id)
			continue
		}
		if tpl.RequiresGPU != tt.want {
			t.Errorf("template %q: RequiresGPU = %v, want %v", tt.id, tpl.RequiresGPU, tt.want)
		}
	}
}

func TestTemplates_NewEntriesPresent(t *testing.T) {
	ids := []string{
		"vllm", "llama-cpp", "litellm", "langfuse", "flowise", "langflow", "weaviate", "chroma",
		"milvus", "speaches", "mlflow", "label-studio", "librechat", "authentik", "temporal", "windmill",
	}
	have := make(map[string]bool, len(Templates))
	for _, tpl := range Templates {
		have[tpl.ID] = true
	}
	for _, id := range ids {
		if !have[id] {
			t.Errorf("missing template %q", id)
		}
	}
}
