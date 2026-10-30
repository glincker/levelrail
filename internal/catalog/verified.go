package catalog

import (
	_ "embed"
	"encoding/json"
)

//go:embed verified.json
var verifiedJSON []byte

// VerifiedRecord is the committed result of the last live smoke pass
// (scripts/catalog-smoke.sh): which templates came up healthy, when, where.
type VerifiedRecord struct {
	VerifiedOn  string   `json:"verified_on"`
	Environment string   `json:"environment"`
	Templates   []string `json:"templates"`
}

var verifiedRecord, verifiedSet = loadVerified()

func loadVerified() (VerifiedRecord, map[string]bool) {
	var rec VerifiedRecord
	if err := json.Unmarshal(verifiedJSON, &rec); err != nil {
		panic("catalog: verified.json: " + err.Error())
	}
	set := make(map[string]bool, len(rec.Templates))
	for _, id := range rec.Templates {
		set[id] = true
	}
	return rec, set
}

// IsVerified reports whether the template passed the last committed smoke run.
func IsVerified(id string) bool { return verifiedSet[id] }

// VerifiedInfo returns the date and environment of the last smoke pass.
func VerifiedInfo() VerifiedRecord { return verifiedRecord }
