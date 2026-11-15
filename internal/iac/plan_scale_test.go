package iac

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// scaleManifest builds n App documents, each with its own Domain document.
func scaleManifest(n int) string {
	var b strings.Builder
	b.WriteString("version: 1\nkind: Project\nmetadata: {name: shop}\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "---\nversion: 1\nkind: App\nmetadata: {name: app-%d}\nspec:\n  project: shop\n  service:\n    build: {type: image, image: \"ghcr.io/acme/web:1.2.3\"}\n    port: 3000\n", i)
		fmt.Fprintf(&b, "---\nversion: 1\nkind: Domain\nmetadata: {name: app-%d.example.com}\nspec: {app: app-%d}\n", i, i)
	}
	return b.String()
}

func TestPlanScalesWithManyApps(t *testing.T) {
	n := 500
	start := time.Now()
	p := planFor(t, newFake(), scaleManifest(n), Options{Prune: true, Source: "scale"})
	elapsed := time.Since(start)
	if p.Summary.Error != 0 {
		t.Fatalf("summary = %+v", p.Summary)
	}
	if p.Summary.Create < n {
		t.Fatalf("create = %d, want at least %d", p.Summary.Create, n)
	}
	if limit := 5 * time.Second; elapsed > limit {
		t.Fatalf("planning %d apps took %s, want under %s", n, elapsed, limit)
	}
}

func BenchmarkPlan500Apps(b *testing.B) {
	docs, issues := ParseDocuments([]Source{{Name: "m.yaml", Data: []byte(scaleManifest(500))}})
	if len(issues) > 0 {
		b.Fatal(issues)
	}
	res, issues := Build(docs, Options{})
	if len(issues) > 0 {
		b.Fatal(issues)
	}
	st, err := Load(b.Context(), newFake(), res, Options{})
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		PlanFor(st, res, Options{})
	}
}
