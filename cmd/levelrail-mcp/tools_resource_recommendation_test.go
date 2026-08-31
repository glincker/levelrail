package main

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestGetResourceRecommendation(t *testing.T) {
	var gotPath string
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.ResourceRecommendationResource{
			ServiceName:    "web",
			LookbackWindow: "168h0m0s",
			Memory: apiclient.DimensionRecommendationResource{
				Dimension: "memory", SampleCount: 200, DataSufficient: true, Confidence: "high",
				Action: "raise", Reason: "p95 usage is close to the current limit",
			},
			CPU: apiclient.DimensionRecommendationResource{Dimension: "cpu", Reason: "not enough data"},
		})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_resource_recommendation",
		Arguments: map[string]any{"name": "web"},
	})
	if err != nil {
		t.Fatalf("CallTool(get_resource_recommendation) error = %v", err)
	}
	if gotPath != "/api/v1/apps/web/resource-recommendation" {
		t.Errorf("path = %q, want /api/v1/apps/web/resource-recommendation", gotPath)
	}

	var rec apiclient.ResourceRecommendationResource
	decodeStructured(t, result, &rec)
	if rec.Memory.Action != "raise" {
		t.Errorf("Memory.Action = %q, want %q", rec.Memory.Action, "raise")
	}
}

func TestGetDatabaseResourceRecommendation(t *testing.T) {
	var gotPath string
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.ResourceRecommendationResource{
			ServiceName:    "main",
			LookbackWindow: "168h0m0s",
			Memory: apiclient.DimensionRecommendationResource{
				Dimension: "memory", SampleCount: 200, DataSufficient: true, Confidence: "high",
				Action: "lower", Reason: "p95 usage is well under the current limit",
			},
			CPU: apiclient.DimensionRecommendationResource{Dimension: "cpu", Reason: "not enough data"},
		})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_database_resource_recommendation",
		Arguments: map[string]any{"name": "main"},
	})
	if err != nil {
		t.Fatalf("CallTool(get_database_resource_recommendation) error = %v", err)
	}
	if gotPath != "/api/v1/databases/main/resource-recommendation" {
		t.Errorf("path = %q, want /api/v1/databases/main/resource-recommendation", gotPath)
	}

	var rec apiclient.ResourceRecommendationResource
	decodeStructured(t, result, &rec)
	if rec.Memory.Action != "lower" {
		t.Errorf("Memory.Action = %q, want %q", rec.Memory.Action, "lower")
	}
}
