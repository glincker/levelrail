package main

import "testing"

func TestParseFixes(t *testing.T) {
	tests := []struct {
		name     string
		in       []string
		wantErr  bool
		wantPort int
		wantName string
		wantDrop bool
	}{
		{name: "port", in: []string{"a1:port=8080"}, wantPort: 8080},
		{name: "name and hold", in: []string{"a1:name=web", "a1:hold-domains"}, wantName: "web", wantDrop: true},
		{name: "bad port", in: []string{"a1:port=99999"}, wantErr: true},
		{name: "no colon", in: []string{"port=1"}, wantErr: true},
		{name: "unknown fix", in: []string{"a1:colour=red"}, wantErr: true},
		{name: "empty name", in: []string{"a1:name="}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseFixes(tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if err != nil {
				return
			}
			ov := got["a1"]
			if ov.Port != tc.wantPort || ov.Name != tc.wantName || ov.DropDomains != tc.wantDrop {
				t.Fatalf("override = %+v", ov)
			}
		})
	}
}

func TestParseVolumeStat(t *testing.T) {
	st, err := parseVolumeStat("data=3:90:3:90")
	if err != nil || st.Name != "data" || st.SourceFiles != 3 || st.TargetBytes != 90 {
		t.Fatalf("got %+v, %v", st, err)
	}
	for _, bad := range []string{"data", "data=1:2:3", "=1:2:3:4", "data=a:2:3:4", "data=-1:2:3:4"} {
		if _, err := parseVolumeStat(bad); err == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
}
