package iac

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCoverageMatrix(t *testing.T) {
	var own struct {
		Props struct {
			Kind struct {
				Enum []string `json:"enum"`
			} `json:"kind"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(resourcesSchema, &own); err != nil {
		t.Fatal(err)
	}
	has := map[Kind]bool{}
	seen := map[string]bool{}
	for _, r := range CoverageMatrix() {
		has[r.Kind] = true
		key := string(r.Kind) + "/" + r.Feature
		if seen[key] {
			t.Errorf("duplicate row %s", key)
		}
		seen[key] = true
		switch r.Status {
		case CoverageInFiles:
			if r.Probe != "" || r.Reason != "" {
				t.Errorf("%s: in-files row carries a rejection", key)
			}
		case CoverageNotYet:
			if r.Reason == "" || r.Probe == "" {
				t.Errorf("%s: not-yet row needs a reason and a probe", key)
			}
			if r.Code != "" && !isKnownCode(r.Code) {
				t.Errorf("%s: unknown code %q", key, r.Code)
			}
		default:
			t.Errorf("%s: bad status %q", key, r.Status)
		}
	}
	for _, k := range own.Props.Kind.Enum {
		if !has[Kind(k)] {
			t.Errorf("schema kind %s has no coverage row", k)
		}
	}
	if len(own.Props.Kind.Enum) != len(Kinds) {
		t.Errorf("schema lists %d kinds, Kinds has %d", len(own.Props.Kind.Enum), len(Kinds))
	}
}

func isKnownCode(c string) bool {
	for _, k := range AllCodes {
		if k == c {
			return true
		}
	}
	return false
}

// A not-yet row is only honest if the product really rejects it.
func TestCoverageMatrixNotYetRowsAreRejected(t *testing.T) {
	for _, r := range CoverageMatrix() {
		if r.Status != CoverageNotYet {
			continue
		}
		t.Run(string(r.Kind)+"/"+r.Feature, func(t *testing.T) {
			docs, issues := ParseDocuments([]Source{{Name: "p.yaml", Data: []byte(r.Probe)}})
			if len(issues) == 0 {
				_, issues = Build(docs, Options{})
			}
			if len(issues) == 0 {
				t.Fatal("probe was accepted")
			}
			found := false
			for _, i := range issues {
				if strings.Contains(i.Message, r.Reason) && (r.Code == "" || i.Code == r.Code) {
					found = true
				}
			}
			if !found {
				t.Fatalf("no issue with reason %q code %q: %+v", r.Reason, r.Code, issues)
			}
		})
	}
}

func TestCoverageMatrixStable(t *testing.T) {
	a, _ := json.Marshal(CoverageMatrix())
	b, _ := json.Marshal(CoverageMatrix())
	if string(a) != string(b) {
		t.Fatal("matrix is not deterministic")
	}
	rank := map[Kind]int{}
	for i, k := range Kinds {
		rank[k] = i
	}
	rows := CoverageMatrix()
	for i := 1; i < len(rows); i++ {
		if rank[rows[i-1].Kind] > rank[rows[i].Kind] {
			t.Errorf("row %d kind %s out of order after %s", i, rows[i].Kind, rows[i-1].Kind)
		}
	}
}
