package build

import (
	"errors"
	"testing"
)

func TestSelectBuildNode(t *testing.T) {
	tests := []struct {
		name    string
		nodes   []NodeInfo
		want    string
		wantErr error
	}{
		{
			name:  "no nodes at all: local",
			nodes: nil,
			want:  "",
		},
		{
			name: "no node is build-capable: local",
			nodes: []NodeInfo{
				{ID: "node_a", AcceptsBuildWorkloads: false, Online: true},
				{ID: "node_b", AcceptsBuildWorkloads: false, Online: true},
			},
			want: "",
		},
		{
			name: "one build-capable node, online: selected",
			nodes: []NodeInfo{
				{ID: "node_a", AcceptsBuildWorkloads: false, Online: true},
				{ID: "node_b", AcceptsBuildWorkloads: true, Online: true},
			},
			want: "node_b",
		},
		{
			name: "multiple build-capable online nodes: deterministic, smallest ID wins",
			nodes: []NodeInfo{
				{ID: "node_z", AcceptsBuildWorkloads: true, Online: true},
				{ID: "node_a", AcceptsBuildWorkloads: true, Online: true},
				{ID: "node_m", AcceptsBuildWorkloads: true, Online: true},
			},
			want: "node_a",
		},
		{
			name: "build-capable node offline, another build-capable node online: the online one wins",
			nodes: []NodeInfo{
				{ID: "node_a", AcceptsBuildWorkloads: true, Online: false},
				{ID: "node_b", AcceptsBuildWorkloads: true, Online: true},
			},
			want: "node_b",
		},
		{
			name: "build-capable node configured but unavailable: explicit error, no silent local fallback",
			nodes: []NodeInfo{
				{ID: "node_a", AcceptsBuildWorkloads: true, Online: false},
			},
			wantErr: ErrNoBuildNodeAvailable,
		},
		{
			name: "every build-capable node offline: explicit error",
			nodes: []NodeInfo{
				{ID: "node_a", AcceptsBuildWorkloads: true, Online: false},
				{ID: "node_b", AcceptsBuildWorkloads: true, Online: false},
				{ID: "node_c", AcceptsBuildWorkloads: false, Online: true},
			},
			wantErr: ErrNoBuildNodeAvailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SelectBuildNode(tt.nodes)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("SelectBuildNode() err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("SelectBuildNode() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("SelectBuildNode() = %q, want %q", got, tt.want)
			}
		})
	}
}
