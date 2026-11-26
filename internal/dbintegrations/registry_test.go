package dbintegrations

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestRegistry_Valid(t *testing.T) {
	cards, err := Cards()
	if err != nil || len(cards) == 0 {
		t.Fatalf("cards = %d err = %v", len(cards), err)
	}
	engines, err := store.SupportedDatabaseEngines()
	if err != nil {
		t.Fatal(err)
	}
	known := map[string]bool{AllEngines: true}
	for _, e := range engines {
		known[e.ID] = true
	}
	cats := map[Category]bool{CategoryMonitor: true, CategoryDevelop: true, CategoryDeploy: true, CategoryQuery: true, CategoryReplicate: true}
	seen := map[string]bool{}
	for _, c := range cards {
		if c.ID == "" || seen[c.ID] {
			t.Errorf("empty or duplicate id %q", c.ID)
		}
		seen[c.ID] = true
		if !cats[c.Category] {
			t.Errorf("%s: unknown category %q", c.ID, c.Category)
		}
		if c.Kind != KindConnector && c.Kind != KindDocs {
			t.Errorf("%s: unknown kind %q", c.ID, c.Kind)
		}
		if c.Title == "" || c.Summary == "" || c.StatusSource == "" || len(c.Engines) == 0 {
			t.Errorf("%s: missing title, summary, status_source or engines", c.ID)
		}
		for _, e := range c.Engines {
			if !known[e] {
				t.Errorf("%s: unknown engine %q", c.ID, e)
			}
		}
		if c.Docs == "" {
			t.Errorf("%s: missing docs slug", c.ID)
			continue
		}
		_, statErr := os.Stat(filepath.Join("..", "..", "docs", c.Docs+".md"))
		if statErr != nil && !c.DocsPlanned {
			t.Errorf("%s: docs page %q missing and not marked docs_planned", c.ID, c.Docs)
		}
	}
}

type fakeLister []store.ExportDestination

func (f fakeLister) List(context.Context) ([]store.ExportDestination, error) { return f, nil }

func TestService_ExportStatusScopedToDatabase(t *testing.T) {
	src := fakeLister{
		{ID: "a", Kind: store.ExportKindLogsDatadog, Enabled: true, Apps: []string{"other"}},
		{ID: "b", Kind: store.ExportKindMetricsOTLP, Enabled: true, Apps: []string{"main"}},
	}
	svc := NewService(nil, NewDatadogProvider(src), NewOpenTelemetryProvider(src))
	list, err := svc.List(context.Background(), Database{Name: "main", Engine: "postgres"})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]StateCode{}
	for _, c := range list {
		got[c.ID] = c.State.Code
	}
	if got["datadog"] != StateAvailable || got["opentelemetry"] != StateConnected {
		t.Errorf("states = %v", got)
	}
	if got["prisma"] != StateDocsOnly || got["hasura"] != StatePlanned {
		t.Errorf("states = %v", got)
	}
	redis, _ := svc.List(context.Background(), Database{Name: "r", Engine: "redis"})
	for _, c := range redis {
		if c.ID == "hasura" && c.State.Code != StateUnavailable {
			t.Errorf("hasura on redis = %v", c.State)
		}
	}
}
