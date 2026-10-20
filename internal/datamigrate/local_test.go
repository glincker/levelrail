package datamigrate

import (
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
)

func TestSelectLocalSource(t *testing.T) {
	tests := []struct {
		name        string
		c           docker.LocalContainer
		wantNetwork string
		wantHost    string
		wantPort    int
		problem     bool
	}{
		{"user network uses name", docker.LocalContainer{Name: "pg", Image: "postgres:16", Running: true,
			Networks: []docker.LocalNetwork{{Name: "bridge", IP: "172.17.0.2"}, {Name: "coolify", IP: "10.0.0.5"}}, Ports: []int{5432}},
			"coolify", "pg", 5432, false},
		{"default bridge falls back to IP", docker.LocalContainer{Name: "pg", Image: "postgres:17-alpine", Running: true,
			Networks: []docker.LocalNetwork{{Name: "bridge", IP: "172.17.0.2"}}}, "bridge", "172.17.0.2", 5432, false},
		{"first user network alphabetically", docker.LocalContainer{Name: "db", Image: "mysql:8", Running: true,
			Networks: []docker.LocalNetwork{{Name: "zeta", IP: "10.1.0.2"}, {Name: "alpha", IP: "10.2.0.2"}}}, "zeta", "db", 3306, false},
		{"non default port exposed", docker.LocalContainer{Name: "pg", Image: "postgres:16", Running: true,
			Networks: []docker.LocalNetwork{{Name: "n", IP: "10.0.0.2"}}, Ports: []int{6543}}, "n", "pg", 6543, false},
		{"stopped", docker.LocalContainer{Name: "pg", Image: "postgres:16"}, "", "", 0, true},
		{"host network only", docker.LocalContainer{Name: "pg", Image: "postgres:16", Running: true,
			Networks: []docker.LocalNetwork{{Name: "host", IP: "127.0.0.1"}}}, "", "", 0, true},
		{"not a database", docker.LocalContainer{Name: "web", Image: "nginx:1", Running: true}, "", "", 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SelectLocalSource(tc.c)
			if (got.Problem != "") != tc.problem {
				t.Fatalf("problem = %q, want problem=%v", got.Problem, tc.problem)
			}
			if tc.problem {
				return
			}
			if got.Network != tc.wantNetwork || got.Host != tc.wantHost || got.Port != tc.wantPort {
				t.Fatalf("got network=%q host=%q port=%d, want %q %q %d", got.Network, got.Host, got.Port, tc.wantNetwork, tc.wantHost, tc.wantPort)
			}
		})
	}
}

func TestHelperJoinsSourceNetwork(t *testing.T) {
	rt := &fakeRuntime{running: true}
	c := &Copier{Runtime: rt, HelperNetwork: "coolify"}
	if _, err := c.startHelper(t.Context(), Target{Name: "x", Engine: EnginePostgres, Version: "16"}, Source{Host: "pg"}); err != nil {
		t.Fatal(err)
	}
	if len(rt.created) != 1 || rt.created[0].Network == nil || rt.created[0].Network.Name != "coolify" {
		t.Fatalf("helper network = %+v", rt.created)
	}
	rt2 := &fakeRuntime{running: true}
	c2 := &Copier{Runtime: rt2, HelperNetwork: "bridge"}
	if _, err := c2.startHelper(t.Context(), Target{Name: "x", Engine: EnginePostgres, Version: "16"}, Source{Host: "172.17.0.2"}); err != nil {
		t.Fatal(err)
	}
	if rt2.created[0].Network != nil {
		t.Fatalf("default bridge must not set an explicit network: %+v", rt2.created[0].Network)
	}
}

func TestExplainReachFailure(t *testing.T) {
	raw := "pg_dump: error: connection to server at \"10.0.0.1\", port 5432 failed: timeout expired"
	if got := ExplainReachFailure(raw, false); !strings.Contains(got, "network isolation") || !strings.Contains(got, raw) {
		t.Errorf("non-local timeout not explained with the raw error kept: %q", got)
	}
	if got := ExplainReachFailure(raw, true); got != raw {
		t.Errorf("local source must keep the raw error: %q", got)
	}
	if got := ExplainReachFailure("password authentication failed", false); got != "password authentication failed" {
		t.Errorf("auth failure was rewritten: %q", got)
	}
}
