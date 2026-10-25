package application

import (
	"errors"
	"testing"
)

func TestCreateFailureNodeRecovering(t *testing.T) {
	bindErr := errors.New("listen tcp 10.181.0.2:8080: bind: cannot assign requested address")
	cases := []struct {
		name       string
		recovering bool
		err        error
		want       string
	}{
		{"recovering node, address not ready", true, bindErr, ReasonNodeRecovering},
		{"steady node, same error", false, bindErr, "CreateFailed"},
		{"recovering node, other error", true, errors.New("no such image"), "CreateFailed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &Controller{nodeRecovering: tc.recovering}
			got, _ := c.createFailure(tc.err)
			if got != tc.want {
				t.Errorf("reason = %q, want %q", got, tc.want)
			}
		})
	}
	_, err := (&Controller{nodeRecovering: true}).createFailure(bindErr)
	if err == nil || err.Error()[:len("node recovered, waiting for the mesh address")] != "node recovered, waiting for the mesh address" {
		t.Errorf("message = %v", err)
	}
}
