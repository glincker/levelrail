package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestNodeGPUColumn(t *testing.T) {
	tests := []struct {
		name string
		gpu  *apiclient.NodeGPUResource
		want string
	}{
		{"no gpu", nil, "-"},
		{"reported absent", &apiclient.NodeGPUResource{}, "-"},
		{"partially reserved", &apiclient.NodeGPUResource{Present: true, RuntimeInstalled: true, GPUCount: 2, ReservedGPUs: 1, FreeGPUs: 1}, "1/2 free"},
		{"exactly full", &apiclient.NodeGPUResource{Present: true, RuntimeInstalled: true, GPUCount: 1, ReservedGPUs: 1}, "0/1 free"},
		{"runtime missing", &apiclient.NodeGPUResource{Present: true, GPUCount: 1, FreeGPUs: 1}, "1/1 free (no nvidia runtime)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nodeGPUColumn(tt.gpu); got != tt.want {
				t.Errorf("nodeGPUColumn = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPrintNodesTable_GPUColumn(t *testing.T) {
	var out bytes.Buffer
	printNodesTable(&out, []nodeResource{{ID: "n1", Name: "gpu-box", Status: "online", GPU: &apiclient.NodeGPUResource{Present: true, RuntimeInstalled: true, GPUCount: 2, ReservedGPUs: 1, FreeGPUs: 1}}})
	if !strings.Contains(out.String(), "GPU") || !strings.Contains(out.String(), "1/2 free") {
		t.Errorf("table = %q", out.String())
	}
}

func TestPrintDrainNodeResultHuman_Blocked(t *testing.T) {
	var out bytes.Buffer
	printDrainNodeResultHuman(&out, "n1", drainNodeResponse{
		Blocked: []apiclient.DrainBlockedResource{{Kind: "app", Name: "llm", Reason: "no GPU node available (n2: not enough free GPUs)"}},
		Errors:  []string{"service llm: no GPU node available (n2: not enough free GPUs)"},
	})
	got := out.String()
	if !strings.Contains(got, "blocked (left on the node)") || !strings.Contains(got, "app llm: no GPU node available") {
		t.Errorf("output = %q", got)
	}
	if strings.Contains(got, "errors:") {
		t.Errorf("blocked entries must not be repeated under errors: %q", got)
	}
}

func TestPrintGPUNodesTable_Reserved(t *testing.T) {
	var out bytes.Buffer
	printGPUNodesTable(&out, []apiclient.GPUNodeResource{{Name: "n1", Present: true, RuntimeInstalled: true, GPUCount: 2, ReservedGPUs: 1, UsedVRAMMiB: 10, TotalVRAMMiB: 20}})
	if !strings.Contains(out.String(), "RESERVED") || !strings.Contains(out.String(), "1/2") {
		t.Errorf("table = %q", out.String())
	}
}
