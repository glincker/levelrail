package api

import (
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/iac"
)

func TestSyncProjectDroppedIsLoud(t *testing.T) {
	doc := "apiVersion: levelrail.dev/v1\nkind: App\nmetadata:\n  name: web\nspec:\n  project: other\n"
	req := stackRequest{Files: []iacFile{{Name: "app.yaml", Content: doc}}}
	ce := syncProjectDropped(req, "mine", 0)
	if ce == nil || ce.Code != iac.CodeSyncPlanFailed || !strings.Contains(ce.Msg, "dropped") {
		t.Fatalf("want a coded plan failure naming the drop, got %+v", ce)
	}
	if syncProjectDropped(req, "mine", 1) != nil || syncProjectDropped(req, "", 0) != nil {
		t.Fatal("a non-empty plan or an unscoped stack must not fail")
	}
}
