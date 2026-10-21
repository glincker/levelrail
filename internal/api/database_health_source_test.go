package api

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestRouterDatabaseUp(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name    string
		state   *docker.ContainerState
		inspErr error
		suspend bool
		wantUp  bool
		wantErr error
	}{
		{"running", &docker.ContainerState{Running: true}, nil, false, true, nil},
		{"stopped is down", &docker.ContainerState{Running: false}, nil, false, false, nil},
		{"unreadable is unavailable", nil, errors.New("offline"), false, false, alerting.ErrDatabaseHealthUnavailable},
		{"suspended is unavailable", &docker.ContainerState{Running: false}, nil, true, false, alerting.ErrDatabaseHealthUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt, db := newTestRouterWithExecRuntime(t, &fakeExecAppRuntime{inspectState: tc.state, inspectErr: tc.inspErr})
			if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "pg", Engine: "postgres", Version: "16"}); err != nil {
				t.Fatal(err)
			}
			if tc.suspend {
				if _, err := db.ExecContext(ctx, `UPDATE desired_databases SET suspended = 1 WHERE name = 'pg'`); err != nil {
					t.Fatalf("suspend: %v", err)
				}
			}
			up, err := rt.DatabaseUp(ctx, "pg")
			if !errors.Is(err, tc.wantErr) || up != tc.wantUp {
				t.Fatalf("up=%v err=%v, want up=%v err=%v", up, err, tc.wantUp, tc.wantErr)
			}
		})
	}
}
