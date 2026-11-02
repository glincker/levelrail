package main

import "testing"

func TestParseAnchorExport_LinesAndArray(t *testing.T) {
	lines := []byte("{\"v\":1,\"seq\":2,\"hash\":\"a\",\"at\":\"t\",\"sig\":\"k:x\"}\n\n{\"v\":1,\"seq\":3,\"hash\":\"b\",\"at\":\"t\",\"sig\":\"k:y\"}\n")
	got, err := parseAnchorExport(lines)
	if err != nil || len(got) != 2 || got[1].Seq != 3 {
		t.Fatalf("lines: %+v err=%v", got, err)
	}
	got, err = parseAnchorExport([]byte(`[{"v":1,"seq":9,"hash":"c","at":"t","sig":"h:z"}]`))
	if err != nil || len(got) != 1 || got[0].Hash != "c" {
		t.Fatalf("array: %+v err=%v", got, err)
	}
	if _, err := parseAnchorExport([]byte("not json")); err == nil {
		t.Fatal("garbage accepted")
	}
	if _, err := parseAnchorExport([]byte("  ")); err == nil {
		t.Fatal("empty export accepted")
	}
}
