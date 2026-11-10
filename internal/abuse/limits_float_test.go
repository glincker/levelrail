package abuse

import "testing"

func TestEnvFloatRejectsNaNAndInf(t *testing.T) {
	for _, v := range []string{"NaN", "nan", "Inf", "+Inf", "-Inf", "1e999", "0", "-1", "x"} {
		got := envFloat(func(string) string { return v }, "K", 7)
		if got != 7 {
			t.Errorf("envFloat(%q) = %v, want the default", v, got)
		}
	}
	if got := envFloat(func(string) string { return "2.5" }, "K", 7); got != 2.5 {
		t.Errorf("envFloat(2.5) = %v", got)
	}
}
