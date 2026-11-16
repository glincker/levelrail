package buildkitd

import "testing"

func TestSpecCarriesPidsLimit(t *testing.T) {
	s := &Supervisor{cfg: Config{Image: DefaultImageRef}, pids: 4096}
	if got := s.spec().PidsLimit; got != 4096 {
		t.Fatalf("pids limit = %d", got)
	}
	if got := (&Supervisor{cfg: Config{Image: DefaultImageRef}}).spec().PidsLimit; got != 0 {
		t.Fatalf("unset pids limit = %d", got)
	}
}
