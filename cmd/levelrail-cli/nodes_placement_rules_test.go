package main

import "testing"

func TestParseTolerations(t *testing.T) {
	got, err := parseTolerations([]string{"dedicated=gpu:NoSchedule", "spot:PreferNoSchedule"}, []string{"infra"})
	if err != nil || len(got) != 3 {
		t.Fatalf("parseTolerations() = %+v, %v", got, err)
	}
	if got[0].Operator != "" || got[0].Value != "gpu" || got[1].Operator != "Exists" || got[2].Key != "infra" {
		t.Fatalf("parsed = %+v", got)
	}
	if _, err := parseTolerations([]string{"nocolon"}, nil); err == nil {
		t.Fatal("want error for missing effect")
	}
}
