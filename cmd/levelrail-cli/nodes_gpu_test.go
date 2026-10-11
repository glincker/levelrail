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

func TestPrintDrainHuman_Blocked(t *testing.T) {
	var out bytes.Buffer
	printDrainHuman(&out, apiclient.NodeDrainResource{ID: "dr1", NodeID: "n1", Status: "failed",
		Blocked: []apiclient.DrainBlocked{{Kind: "app", Name: "llm", Reason: "no GPU node available (n2: not enough free GPUs)"}},
		Error:   "could not move: app llm"}, true)
	got := out.String()
	if !strings.Contains(got, "blocked: app llm: no GPU node available") || !strings.Contains(got, "error: could not move") {
		t.Errorf("output = %q", got)
	}
}

func TestPrintGPUNodesTable_Reserved(t *testing.T) {
	var out bytes.Buffer
	printGPUNodesTable(&out, []apiclient.GPUNodeResource{{Name: "n1", Present: true, RuntimeInstalled: true, GPUCount: 2, ReservedGPUs: 1, UsedVRAMMiB: 10, TotalVRAMMiB: 20}})
	if !strings.Contains(out.String(), "RESERVED") || !strings.Contains(out.String(), "1/2") {
		t.Errorf("table = %q", out.String())
	}
}
