package runtimesettings

import "testing"

func TestPruneScheduleCheck(t *testing.T) {
	cases := []struct {
		in      string
		wantErr bool
	}{
		{"off", false},
		{"0 3 * * 0", false},
		{"0 0 30 2 *", true},
		{"garbage", true},
	}
	for _, tc := range cases {
		if err := pruneScheduleCheck(tc.in); (err != nil) != tc.wantErr {
			t.Errorf("%q: err = %v, wantErr %v", tc.in, err, tc.wantErr)
		}
	}
}
