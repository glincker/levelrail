package rollbackref

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

type inspector struct{ id string }

func (i inspector) InspectImageID(context.Context, string) (string, error) { return i.id, nil }

func TestResolve(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	local := store.DeployAttempt{Status: store.DeployAttemptStatusSucceeded, Image: "web:abc", ImageDigest: "sha256:local", DigestReason: store.DigestReasonLocalBuild}
	cases := []struct {
		name   string
		insp   inspector
		status int
	}{
		{"matching local image", inspector{id: "sha256:local"}, 0},
		{"collected image", inspector{}, http.StatusGone},
		{"moved tag", inspector{id: "sha256:other"}, http.StatusConflict},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, status, _ := Resolve(context.Background(), c.insp, log, local, nil)
			if status != c.status {
				t.Fatalf("status = %d, want %d", status, c.status)
			}
		})
	}
}
