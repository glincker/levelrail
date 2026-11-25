package runtimesettings

import "testing"

func TestBranchHeadroomCheck(t *testing.T) {
	for _, v := range []string{"NaN", "nan", "+Inf", "Inf", "0.5", "11", "x", ""} {
		if branchHeadroomCheck(v) == nil {
			t.Errorf("%q accepted", v)
		}
	}
	for _, v := range []string{"1", "1.5", "10"} {
		if err := branchHeadroomCheck(v); err != nil {
			t.Errorf("%q refused: %v", v, err)
		}
	}
}

func TestBranchDurationBounds(t *testing.T) {
	var d Definition
	for _, x := range Definitions() {
		if x.Key == KeyBranchesExpiryInterval {
			d = x
		}
	}
	if d.CheckValue == nil {
		t.Fatal("key missing")
	}
	for _, v := range []string{"0s", "30s", "-5m", "25h", "soon"} {
		if d.CheckValue(v) == nil {
			t.Errorf("%q accepted", v)
		}
	}
	if err := d.CheckValue("5m"); err != nil {
		t.Errorf("5m refused: %v", err)
	}
}

func TestBranchEnvSeedIsChecked(t *testing.T) {
	for _, x := range Definitions() {
		if len(x.Key) < 9 || x.Key[:9] != "branches." || x.Kind == KindBool {
			continue
		}
		if x.EnvConvert == nil {
			t.Errorf("%s has no env check", x.Key)
		}
	}
	for _, x := range Definitions() {
		switch x.Key {
		case KeyBranchesExpiryInterval, KeyBranchesDefaultTTL:
			if _, err := x.EnvConvert("0s"); err == nil {
				t.Errorf("%s accepted 0s from env", x.Key)
			}
		case KeyBranchesMaxPerRoot:
			if _, err := x.EnvConvert("0"); err == nil {
				t.Errorf("%s accepted 0 from env", x.Key)
			}
		}
	}
}
