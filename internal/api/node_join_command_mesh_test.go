package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// A mesh-enabled control plane always prints mesh-ready join commands, even
// without --mesh, or the agent enrolls but refuses its mesh config.
func TestJoinTokenIncludesMeshWhenControlPlaneMeshIsOn(t *testing.T) {
	for _, tt := range []struct {
		name     string
		meshOn   bool
		body     string
		wantMesh bool
	}{
		{name: "mesh on, no flag", meshOn: true, body: `{"control_plane_addr":"cp.example.com:9443"}`, wantMesh: true},
		{name: "mesh off, flag", meshOn: false, body: `{"control_plane_addr":"cp.example.com:9443","mesh":true}`, wantMesh: true},
		{name: "mesh off, no flag", meshOn: false, body: `{"control_plane_addr":"cp.example.com:9443"}`, wantMesh: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rt, db := newTestRouter(t)
			if tt.meshOn {
				rt.mesh = &fakeMeshStatus{}
			}
			cookie := loginTestSession(t, rt, db)
			rec := mnDo(t, rt, cookie, http.MethodPost, "/api/v1/nodes/join-tokens", tt.body)
			if rec.Code != http.StatusCreated {
				t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
			}
			var got createNodeJoinTokenResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			cmds := got.JoinCommands
			checks := map[string][]string{
				"binary": {cmds.Binary, "APP_MESH_ENABLED=1"},
				"docker": {cmds.Docker, "APP_MESH_ENABLED=1", "--cap-add NET_ADMIN", "--device /dev/net/tun"},
				"podman": {cmds.Podman, "APP_MESH_ENABLED=1", "--cap-add NET_ADMIN", "--device /dev/net/tun"},
			}
			for kind, c := range checks {
				for _, want := range c[1:] {
					if strings.Contains(c[0], want) != tt.wantMesh {
						t.Errorf("%s command contains %q = %v, want %v: %s", kind, want, !tt.wantMesh, tt.wantMesh, c[0])
					}
				}
			}
		})
	}
}
