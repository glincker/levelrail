package api

import (
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/iac"
)

func TestSyncProjectDroppedIsLoud(t *testing.T) {
	doc := "apiVersion: levelrail.dev/v1\nkind: App\nmetadata:\n  name: web\nspec:\n  project: other\n"
	req := stackRequest{Files: []iacFile{{Name: "app.yaml", Content: doc}}}
	ce := syncProjectDropped(req, "mine", 0, 0)
	if ce == nil || ce.Code != iac.CodeSyncPlanFailed || !strings.Contains(ce.Msg, "dropped") {
		t.Fatalf("want a coded plan failure naming the drop, got %+v", ce)
	}
	if syncProjectDropped(req, "mine", 1, 0) != nil || syncProjectDropped(req, "", 0, 0) != nil {
		t.Fatal("a non-empty plan or an unscoped stack must not fail")
	}
}

func TestSyncProjectDroppedDetailIsBounded(t *testing.T) {
	var files []iacFile
	for _, n := range []string{"a", "b", "c"} {
		files = append(files, iacFile{Name: n + ".yaml", Content: "apiVersion: levelrail.dev/v1\nkind: App\nmetadata:\n  name: " + n + "\nspec:\n  project: other\n"})
	}
	ce := syncProjectDropped(stackRequest{Files: files}, "mine", 0, 1)
	if ce == nil || ce.Detail == "" || !strings.Contains(ce.Detail, "App/") || !strings.Contains(ce.Detail, "and 2 more") {
		t.Fatalf("detail must name the drop and stay bounded, got %+v", ce)
	}
}
