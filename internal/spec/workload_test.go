package spec

import (
	"strings"
	"testing"
)

func TestWorkloadSpec(t *testing.T) {
	base := "version: 1\nservices:\n  db:\n    build:\n      type: image\n      image: pg:16\n    port: 5432\n"
	tests := []struct {
		name    string
		extra   string
		wantErr string
	}{
		{name: "default is accepted", extra: ""},
		{name: "per-node ok", extra: "    workload: per-node\n    perNode:\n      onDrain: evict\n"},
		{name: "ordered ok", extra: "    workload: ordered\n    ordered:\n      replicas: 3\n      start: parallel\n      partition: 1\n"},
		{name: "ordered zero surge ok", extra: "    workload: ordered\n    ordered: {replicas: 2}\n    rollout: {max_surge: 0, max_unavailable: 1}\n"},
		{name: "unknown kind", extra: "    workload: daemon\n", wantErr: "workload"},
		{name: "per-node refuses replicas", extra: "    workload: per-node\n    replicas: 2\n", wantErr: "replicas"},
		{name: "per-node refuses strategy", extra: "    workload: per-node\n    strategy: rolling\n", wantErr: "strategy"},
		{name: "per-node refuses domains", extra: "    workload: per-node\n    domains: [a.example.com]\n", wantErr: "domains"},
		{name: "ordered refuses domains", extra: "    workload: ordered\n    ordered: {replicas: 2}\n    domains: [a.example.com]\n", wantErr: "domains"},
		{name: "surge refused", extra: "    workload: ordered\n    ordered: {replicas: 2}\n    rollout: {max_surge: 1}\n", wantErr: "max_surge"},
		{name: "percent surge refused", extra: "    workload: per-node\n    rollout: {max_surge: 25%}\n", wantErr: "max_surge"},
		{name: "ordered needs replicas", extra: "    workload: ordered\n", wantErr: "ordered.replicas"},
		{name: "ordered bad start", extra: "    workload: ordered\n    ordered: {replicas: 2, start: fast}\n", wantErr: "start"},
		{name: "partition above replicas", extra: "    workload: ordered\n    ordered: {replicas: 2, partition: 3}\n", wantErr: "partition"},
		{name: "perNode needs kind", extra: "    perNode: {onDrain: keep}\n", wantErr: "perNode requires"},
		{name: "ordered needs kind", extra: "    ordered: {replicas: 2}\n", wantErr: "ordered requires"},
		{name: "bad onDrain", extra: "    workload: per-node\n    perNode: {onDrain: drop}\n", wantErr: "onDrain"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(base + tt.extra))
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestWorkloadSpecLongName(t *testing.T) {
	name := strings.Repeat("a", maxWorkloadParentName+1)
	doc := "version: 1\nservices:\n  " + name + ":\n    build:\n      type: image\n      image: pg:16\n    port: 80\n    workload: per-node\n"
	if _, err := Parse([]byte(doc)); err == nil || !strings.Contains(err.Error(), "at most") {
		t.Fatalf("error = %v, want name length refusal", err)
	}
}
