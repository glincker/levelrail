package compose

import "testing"

func TestMaxShmBytes_InvalidEnvKeepsDefault(t *testing.T) {
	for _, raw := range []string{"2g", "0", "-5", "NaN", "lots"} {
		t.Setenv(envMaxShmBytes, raw)
		if got := MaxShmBytes(); got != defaultMaxShmBytes {
			t.Errorf("%s=%q: MaxShmBytes = %d, want default %d", envMaxShmBytes, raw, got, int64(defaultMaxShmBytes))
		}
		if _, err := parseByteSize("100g", envMaxShmBytes); err == nil {
			t.Errorf("%s=%q: shm_size 100g accepted, want the default cap to refuse it", envMaxShmBytes, raw)
		}
	}
	t.Setenv(envMaxShmBytes, "1073741824")
	if got := MaxShmBytes(); got != 1<<30 {
		t.Fatalf("valid cap = %d, want %d", got, 1<<30)
	}
}
