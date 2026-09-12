package build

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

// stubNodeBuilder stands in for a real agent transport: Router only needs
// to know that something can run a build on a node, never how.
type stubNodeBuilder struct {
	calls []string
	req   RemoteRequest
	err   error
}

func (s *stubNodeBuilder) BuildOnNode(_ context.Context, nodeID string, req RemoteRequest, _ io.Writer, _ func(ProgressEvent)) (*Result, error) {
	s.calls = append(s.calls, nodeID)
	s.req = req
	return nil, s.err
}

func TestRouter_SelectNode(t *testing.T) {
	errNodes := errors.New("store is down")

	tests := []struct {
		name    string
		nodes   NodeSource
		remote  NodeBuilder
		want    string
		wantErr error
	}{
		{
			name:   "no node source pins every build local",
			remote: &stubNodeBuilder{},
			want:   "",
		},
		{
			name:  "no dispatcher pins every build local",
			nodes: staticNodes(NodeInfo{ID: "n1", AcceptsBuildWorkloads: true, Online: true}),
			want:  "",
		},
		{
			name:   "no build-capable node keeps building local",
			nodes:  staticNodes(NodeInfo{ID: "n1", Online: true}),
			remote: &stubNodeBuilder{},
			want:   "",
		},
		{
			name:   "an online build node is selected",
			nodes:  staticNodes(NodeInfo{ID: "n2", AcceptsBuildWorkloads: true, Online: true}, NodeInfo{ID: "n1", Online: true}),
			remote: &stubNodeBuilder{},
			want:   "n2",
		},
		{
			name:    "a build node that is offline fails rather than falling back",
			nodes:   staticNodes(NodeInfo{ID: "n2", AcceptsBuildWorkloads: true}),
			remote:  &stubNodeBuilder{},
			wantErr: ErrNoBuildNodeAvailable,
		},
		{
			name:    "a node lookup failure is not silently treated as no build nodes",
			nodes:   func(context.Context) ([]NodeInfo, error) { return nil, errNodes },
			remote:  &stubNodeBuilder{},
			wantErr: errNodes,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRouter(nil, tt.nodes, tt.remote)
			got, err := r.selectNode(t.Context())
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("selectNode() err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("selectNode() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("selectNode() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestRouter_Build_DispatchFailureNamesTheNode covers the error path a
// deploy actually sees when a dispatched build fails: the node that ran
// it has to be in the message, since "the build failed" reads very
// differently when it failed somewhere other than where you are looking.
func TestRouter_Build_DispatchFailureNamesTheNode(t *testing.T) {
	remote := &stubNodeBuilder{err: errors.New("buildkit refused the solve")}
	r := dispatchRouter(remote, staticNodes(NodeInfo{ID: "builder-1", AcceptsBuildWorkloads: true, Online: true}))

	_, err := r.Build(t.Context(), Request{ContextDir: t.TempDir(), Tag: "app:sha"}, nil)
	if err == nil {
		t.Fatal("Build() error = nil, want the dispatched build's failure")
	}
	if got := err.Error(); !strings.Contains(got, "builder-1") || !strings.Contains(got, "buildkit refused the solve") {
		t.Errorf("Build() error = %q, want it to name both the node and the remote failure", got)
	}
	if len(remote.calls) != 1 || remote.calls[0] != "builder-1" {
		t.Errorf("dispatched to %v, want exactly one build on builder-1", remote.calls)
	}
	if remote.req.Kind != RemoteKindDockerfile || remote.req.Tag != "app:sha" {
		t.Errorf("dispatched request = %+v, want a dockerfile build of app:sha", remote.req)
	}
}

// TestRouter_BuildRailpack_Dispatches proves the railpack path routes the
// same way the dockerfile path does, rather than silently staying local.
func TestRouter_BuildRailpack_Dispatches(t *testing.T) {
	remote := &stubNodeBuilder{err: errors.New("no supported provider")}
	r := dispatchRouter(remote, staticNodes(NodeInfo{ID: "builder-1", AcceptsBuildWorkloads: true, Online: true}))

	if _, err := r.BuildRailpack(t.Context(), RailpackRequest{SourceDir: t.TempDir(), Tag: "app:sha"}, nil); err == nil {
		t.Fatal("BuildRailpack() error = nil, want the dispatched build's failure")
	}
	if remote.req.Kind != RemoteKindRailpack {
		t.Errorf("dispatched kind = %q, want %q", remote.req.Kind, RemoteKindRailpack)
	}
}

func staticNodes(nodes ...NodeInfo) NodeSource {
	return func(context.Context) ([]NodeInfo, error) { return nodes, nil }
}

// dispatchRouter wires a Router whose image-load step just drains the
// stream, so a dispatch decision can be asserted without a live daemon.
func dispatchRouter(remote NodeBuilder, nodes NodeSource) *Router {
	r := NewRouter(&Client{}, nodes, remote)
	r.load = func(_ context.Context, tar io.Reader, _ string, _ func(ProgressEvent)) error {
		_, err := io.Copy(io.Discard, tar)
		return err
	}
	return r
}
