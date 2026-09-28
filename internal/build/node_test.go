package build

import "testing"

func TestSelectBuildNode(t *testing.T) {
	tests := []struct {
		name  string
		nodes []NodeInfo
		want  string
	}{
		{
			name:  "no nodes at all: local (single node unchanged)",
			nodes: nil,
			want:  "",
		},
		{
			name: "no node is build-capable: local (single node unchanged)",
			nodes: []NodeInfo{
				{ID: "node_a", AcceptsBuildWorkloads: false, Online: true},
				{ID: "node_b", AcceptsBuildWorkloads: false, Online: true},
			},
			want: "",
		},
		{
			name: "build-only secondary preferred over a marked-no-build primary",
			nodes: []NodeInfo{
				{ID: "primary", AcceptsBuildWorkloads: false, Online: true},
				{ID: "secondary", AcceptsBuildWorkloads: true, Online: true},
			},
			want: "secondary",
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
			name: "the only build-capable node is unhealthy: falls back to the primary rather than failing the build",
			nodes: []NodeInfo{
				{ID: "node_a", AcceptsBuildWorkloads: true, Online: false},
			},
			want: "",
		},
		{
			name: "every build-capable node offline: falls back to the primary rather than failing the build",
			nodes: []NodeInfo{
				{ID: "node_a", AcceptsBuildWorkloads: true, Online: false},
				{ID: "node_b", AcceptsBuildWorkloads: true, Online: false},
				{ID: "node_c", AcceptsBuildWorkloads: false, Online: true},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SelectBuildNode(tt.nodes); got != tt.want {
				t.Errorf("SelectBuildNode() = %q, want %q", got, tt.want)
			}
		})
	}
}
