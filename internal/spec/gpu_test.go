package spec

import (
	"reflect"
	"testing"
)

func TestParse_ResourcesGPU(t *testing.T) {
	base := "version: 1\nservices:\n  llm:\n    build:\n      type: image\n      image: vllm/vllm-openai:latest\n    port: 8000\n    resources:\n      gpu: "
	tests := []struct {
		name    string
		gpu     string
		want    *GPU
		wantErr bool
	}{
		{name: "all", gpu: "all\n", want: &GPU{Count: GPUAll}},
		{name: "count", gpu: "2\n", want: &GPU{Count: 2}},
		{name: "object count all", gpu: "\n        count: all\n", want: &GPU{Count: GPUAll}},
		{name: "object devices", gpu: "\n        devices: [\"0\", \"GPU-abc\"]\n", want: &GPU{Devices: []string{"0", "GPU-abc"}}},
		{name: "zero rejected", gpu: "0\n", wantErr: true},
		{name: "bad word rejected", gpu: "some\n", wantErr: true},
		{name: "unknown key rejected", gpu: "\n        vram: 10\n", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse([]byte(base + tt.gpu))
			if tt.wantErr {
				if err == nil {
					t.Fatal("Parse() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if g := got.Services["llm"].Resources.GPU; !reflect.DeepEqual(g, tt.want) {
				t.Errorf("GPU = %+v, want %+v", g, tt.want)
			}
		})
	}
}
