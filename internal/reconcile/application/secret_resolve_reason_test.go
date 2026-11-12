package application

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestCreateFailureReasonNamesSecretResolution(t *testing.T) {
	err := fmt.Errorf("resolve env: %w", &secretResolveError{err: errors.New(`env var "X" (secret reference secret://bao/app#k): denied`)})
	if got := createFailureReason(err); got != reasonSecretResolveFailed {
		t.Fatalf("reason = %q", got)
	}
	if !strings.Contains(err.Error(), "secret://bao/app#k") {
		t.Fatalf("message lost the reference: %v", err)
	}
	if got := createFailureReason(errors.New("boom")); got != "CreateFailed" {
		t.Fatalf("plain reason = %q", got)
	}
}
