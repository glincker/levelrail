package runtimesettings

import "testing"

func TestDecimalCheck_RejectsNonFiniteAndNegative(t *testing.T) {
	check := decimalCheck(false)
	for _, v := range []string{"NaN", "nan", "Inf", "+Inf", "-Inf", "-1", "1e7", "abc", ""} {
		if check(v) == nil {
			t.Errorf("%q accepted", v)
		}
	}
	for _, v := range []string{"0", "1.10", "25"} {
		if err := check(v); err != nil {
			t.Errorf("%q rejected: %v", v, err)
		}
	}
	if decimalCheck(true)("") != nil {
		t.Error("empty must be allowed when allowEmpty")
	}
}
