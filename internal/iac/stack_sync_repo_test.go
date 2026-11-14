package iac

import "testing"

func TestStackSync_RepoURLRefusals(t *testing.T) {
	for _, raw := range []string{
		"https://git.example.invalid/o/../r.git",
		"https://git.example.invalid/o/r.git?x=1",
		"https://git.example.invalid/o/r.git#frag",
		"https://git.example.invalid/o/r.git?",
		"git@git.example.invalid:o/../r.git",
		"https://user:pw@git.example.invalid/o/r.git",
	} {
		if _, err := (StackSync{RepoURL: raw, Branch: "main"}).Validate(); err == nil {
			t.Errorf("%q was accepted", raw)
		}
	}
	for _, raw := range []string{"https://git.example.invalid/o/r.git", "ssh://git@git.example.invalid/o/r.git", "git@git.example.invalid:o/r.git"} {
		if code, err := (StackSync{RepoURL: raw, Branch: "main"}).Validate(); err != nil {
			t.Errorf("%q refused: %s %v", raw, code, err)
		}
	}
}
