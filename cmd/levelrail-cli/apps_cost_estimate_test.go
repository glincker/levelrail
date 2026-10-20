package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestAppsCostEstimate_JSON(t *testing.T) {
	name := "web"
	var gotPath string
	srv := newListEchoServer(t, &gotPath, costEstimateResource{
		ServiceName: name,
		VCPUCores:   1,
		MemoryGiB:   1,
		CPUBasis:    "declared",
		MemoryBasis: "declared",
		Providers: []costEstimateProviderResource{
			{Key: "vm", Label: "Generic cloud VM", CPUCostUSD: 4, MemoryCostUSD: 4, TotalUSD: 8},
			{Key: "paas", Label: "Generic metered container PaaS", CPUCostUSD: 25, MemoryCostUSD: 10, TotalUSD: 35},
		},
		Note: "Estimate only, not a real bill.",
	})
	t.Cleanup(srv.Close)

	stdout, _ := runCLIExpectOK(t, []string{"apps", "cost", name, "--api-url", srv.URL, "--json"})
	if !strings.Contains(stdout, `"total_usd": 35`) {
		t.Errorf("stdout = %q, want it to contain the paas total_usd field", stdout)
	}
	wantPath := fmt.Sprintf("/api/v1/apps/%s/cost-estimate", name)
	if gotPath != wantPath {
		t.Errorf("request path = %q, want %s", gotPath, wantPath)
	}
}

func TestAppsCostEstimate_Human(t *testing.T) {
	name := "web"
	srv := newListEchoServer(t, nil, costEstimateResource{
		ServiceName: name,
		VCPUCores:   1,
		MemoryGiB:   1,
		CPUBasis:    "declared",
		MemoryBasis: "declared",
		Providers: []costEstimateProviderResource{
			{Key: "vm", Label: "Generic cloud VM", CPUCostUSD: 4, MemoryCostUSD: 4, TotalUSD: 8},
		},
		Note: "Estimate only, not a real bill.",
	})
	t.Cleanup(srv.Close)

	stdout, _ := runCLIExpectOK(t, []string{"apps", "cost", name, "--api-url", srv.URL})
	if !strings.Contains(stdout, "Generic cloud VM") || !strings.Contains(stdout, "$8.00/mo") {
		t.Errorf("stdout = %q, want it to contain the provider label and total", stdout)
	}
	if !strings.Contains(stdout, "Estimate only, not a real bill.") {
		t.Errorf("stdout = %q, want it to contain the estimate disclaimer", stdout)
	}
}
