package api

import "testing"

func TestOffsiteEncryptionRule(t *testing.T) {
	cases := []struct {
		name  string
		in    postureInput
		want  string
		count int
	}{
		{"unread", postureInput{}, postureStatusUnknown, 0},
		{"none configured", postureInput{OffsiteKnown: true}, postureStatusPass, 0},
		{"all encrypted", postureInput{OffsiteKnown: true}, postureStatusPass, 0},
		{"one plain", postureInput{OffsiteKnown: true, PlainOffsite: []string{"r2-main"}}, postureStatusFail, 1},
		{"two plain", postureInput{OffsiteKnown: true, PlainOffsite: []string{"b", "a"}}, postureStatusFail, 2},
	}
	for _, c := range cases {
		it := offsiteEncryptionRule(c.in)
		if it.Status != c.want || it.Count != c.count || it.ID != "offsite_backup_unencrypted" {
			t.Fatalf("%s: got %+v", c.name, it)
		}
	}
}
