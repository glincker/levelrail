package compose

import "testing"

func TestEnvCapRejectsFractionalValues(t *testing.T) {
	t.Setenv("TEST_CAP", "1500.5")
	if got := envCap("TEST_CAP", 100); got != 100 {
		t.Fatalf("fractional cap must fall back, got %v", got)
	}
	t.Setenv("TEST_CAP", "50")
	if got := envCap("TEST_CAP", 100); got != 50 {
		t.Fatalf("whole cap must apply, got %v", got)
	}
	t.Setenv("TEST_CAP", "0.5")
	if v, ok := envCapFloat("TEST_CAP", 4); !ok || v != 0.5 {
		t.Fatalf("cpu cap keeps fractions, got %v %v", v, ok)
	}
}
