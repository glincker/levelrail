package catalog

import (
	_ "embed"
	"encoding/json"
)

//go:embed tombstones.json
var tombstonesJSON []byte

// Tombstones returns removed ids that must not be reused.
func Tombstones() []TombstoneEntry {
	out := []TombstoneEntry{}
	if err := json.Unmarshal(tombstonesJSON, &out); err != nil {
		panic("catalog: tombstones.json: " + err.Error())
	}
	return out
}

//go:embed images.lock.json
var lockJSON []byte

// SignedBy is a verified keyless signature identity.
type SignedBy struct {
	Method   string `json:"method"`
	Identity string `json:"identity"`
}

// LockedImage is what CI resolved for one pinned ref. It is generated, never hand-written.
type LockedImage struct {
	Digest     string    `json:"digest"`
	Signed     *SignedBy `json:"signed"`
	Provenance bool      `json:"provenance"`
	SBOM       bool      `json:"sbom"`
	NonRoot    bool      `json:"non_root"`
}

// Admission is the result of the admission checks for one template.
type Admission struct {
	KEVHit         bool   `json:"kev_hit"`
	SecretsAllowed int    `json:"secrets_allowed"`
	RunURL         string `json:"run_url"`
}

// LockFile is images.lock.json: digests and signals per ref, admission per template.
type LockFile struct {
	Images    map[string]LockedImage `json:"images"`
	Admission map[string]Admission   `json:"admission"`
}

var lockData = loadLock()

func loadLock() LockFile {
	l := LockFile{Images: map[string]LockedImage{}, Admission: map[string]Admission{}}
	if err := json.Unmarshal(lockJSON, &l); err != nil {
		panic("catalog: images.lock.json: " + err.Error())
	}
	return l
}

// Lock returns the committed image lock.
func Lock() LockFile { return lockData }
