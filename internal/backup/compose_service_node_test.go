package backup

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

type nodeRuntime struct {
	docker.Runtime
	name   string
	listed bool
	execed bool
}

func (n *nodeRuntime) ListByPrefix(context.Context, string) ([]docker.ContainerState, error) {
	n.listed = true
	return []docker.ContainerState{{Name: "shop-db-abc", Running: true}}, nil
}

func (n *nodeRuntime) Exec(context.Context, string, []string) (io.ReadCloser, error) {
	n.execed = true
	return io.NopCloser(strings.NewReader("dump")), nil
}

type listerFake struct{ svcs []store.DesiredService }

func (l listerFake) ListServicesByApp(context.Context, string) ([]store.DesiredService, error) {
	return l.svcs, nil
}

func TestComposeResolverAndDumperUseServiceNode(t *testing.T) {
	local, remote := &nodeRuntime{name: "local"}, &nodeRuntime{name: "n2"}
	pick := func(id string) (docker.Runtime, error) {
		if id == "n2" {
			return remote, nil
		}
		return local, nil
	}
	res := &DesiredComposeResolver{
		Services: listerFake{svcs: []store.DesiredService{{Name: "app1-db", NodeID: "n2"}}},
		Runtimes: pick,
		Owns:     func(string, string) bool { return true },
	}
	ctx, container, err := resolveComposeWithNode(context.Background(), res, "app1", "db")
	if err != nil || container != "shop-db-abc" {
		t.Fatalf("resolve = %q, %v", container, err)
	}
	if !remote.listed || local.listed {
		t.Fatalf("listed remote=%v local=%v, want only the service node", remote.listed, local.listed)
	}
	rc, err := (&ComposeServiceDumper{Runtimes: pick}).Dump(ctx, store.EnginePostgres, container)
	if err != nil {
		t.Fatalf("dump: %v", err)
	}
	_ = rc.Close()
	if !remote.execed || local.execed {
		t.Fatalf("exec remote=%v local=%v, want only the service node", remote.execed, local.execed)
	}
}

func TestComposePostgresPreludeClearsAllUserSchemas(t *testing.T) {
	for _, want := range []string{"BEGIN;", "pg_catalog.format('DROP SCHEMA IF EXISTS %I CASCADE', s)", "pg_catalog.pg_namespace", "pg_catalog.pg_depend", "!~ '^pg_'", "'information_schema'", "CREATE SCHEMA public"} {
		if !strings.Contains(composePostgresPrelude, want) {
			t.Fatalf("prelude missing %q", want)
		}
	}
	if strings.Contains(composePostgresPrelude, "DROP SCHEMA public CASCADE") {
		t.Fatal("prelude still resets only schema public")
	}
}
