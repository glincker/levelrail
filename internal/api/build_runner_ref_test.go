package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/GLINCKER/levelrail/internal/build"
)

func TestBuildRunnerSetAuthorizesNodeRef(t *testing.T) {
	rt, db, _ := newPipelineRouter(t)
	rt.buildPerf = db
	rt.SetBuildRunnerNodes(func(context.Context) ([]build.NodeInfo, error) {
		return []build.NodeInfo{{ID: "n-ok", Name: "n-ok", AcceptsBuildWorkloads: true}, {ID: "n-deny", Name: "n-deny", AcceptsBuildWorkloads: true}}, nil
	})
	cookie := bodyRefFixture(t, rt, db)
	runBodyRefCases(t, rt, cookie, []bodyRefCase{
		{"denied node", http.MethodPut, "/api/v1/apps/web/build-runner", `{"node_id":"n-deny","pinned":true}`, http.StatusNotFound, "node not found"},
		{"unknown node", http.MethodPut, "/api/v1/apps/web/build-runner", `{"node_id":"n-nope"}`, http.StatusNotFound, "node not found"},
		{"allowed node", http.MethodPut, "/api/v1/apps/web/build-runner", `{"node_id":"n-ok","pinned":true}`, http.StatusOK, `"n-ok"`},
	})
}
