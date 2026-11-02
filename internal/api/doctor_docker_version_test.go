package api

import (
	"context"
	"strings"
	"testing"
)

type fakeVersionPinger struct{ version string }

func (fakeVersionPinger) Ping(context.Context) error { return nil }
func (f fakeVersionPinger) ServerVersion(context.Context) (string, error) {
	return f.version, nil
}

func TestDoctorCheckDockerVersion(t *testing.T) {
	t.Setenv("APP_READINESS_MIN_DOCKER_MAJOR", "")
	cases := []struct {
		version string
		want    string
	}{
		{"20.10.7", doctorStatusWarn},
		{"24.0.0", doctorStatusOK},
		{"27.3.1", doctorStatusOK},
		{"weird", doctorStatusUnknown},
	}
	for _, c := range cases {
		rt := &Router{dockerPinger: fakeVersionPinger{version: c.version}}
		got := rt.doctorCheckDockerVersion(context.Background())
		if got.Status != c.want {
			t.Errorf("%s: status %v, want %v (%s)", c.version, got.Status, c.want, got.Message)
		}
	}
	rt := &Router{dockerPinger: fakeVersionPinger{version: "20.10.7"}}
	if msg := rt.doctorCheckDockerVersion(context.Background()).Message; !strings.Contains(msg, "20.10.7") || !strings.Contains(msg, "24") {
		t.Errorf("warning must name both numbers: %s", msg)
	}
	if got := (&Router{}).doctorCheckDockerVersion(context.Background()); got.Status != doctorStatusUnknown {
		t.Errorf("no pinger: %v", got.Status)
	}
}
