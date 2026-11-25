package buildkitd

import "testing"

func TestSpecRestartsWithDaemon(t *testing.T) {
	s := &Supervisor{cfg: Config{Image: DefaultImageRef}}
	if got := s.spec().RestartPolicy; got != "unless-stopped" {
		t.Fatalf("restart policy %q, want unless-stopped", got)
	}
}
