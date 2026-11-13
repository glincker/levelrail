package iac

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"sync"
)

// HashV2Prefix marks a plan hash computed by planHashV2.
const HashV2Prefix = "v2:"

const planHashSchema = 2

var (
	hashKeyMu sync.RWMutex
	hashKey   []byte
)

// SetPlanHashKey sets the HMAC key for plan hashes; the caller derives it
// from the server master key so hashes are per install.
func SetPlanHashKey(key []byte) {
	hashKeyMu.Lock()
	hashKey = append([]byte(nil), key...)
	hashKeyMu.Unlock()
}

// currentHashKey falls back to a random per-process key so an unset key
// never yields a guessable hash; plans then only match within one process.
func currentHashKey() []byte {
	hashKeyMu.RLock()
	k := hashKey
	hashKeyMu.RUnlock()
	if len(k) > 0 {
		return k
	}
	hashKeyMu.Lock()
	defer hashKeyMu.Unlock()
	if len(hashKey) == 0 {
		b := make([]byte, 32)
		_, _ = rand.Read(b)
		hashKey = b
	}
	return hashKey
}

type hashItem struct {
	Key           string `json:"key"`
	Action        Action `json:"action"`
	Code          string `json:"code,omitempty"`
	DesiredDigest string `json:"desired"`
	LiveDigest    string `json:"live"`
	// Cloud is set for cloud items only so other plans keep their hash.
	Cloud string `json:"cloud,omitempty"`
}

type hashBody struct {
	SchemaVersion int        `json:"schemaVersion"`
	Stack         string     `json:"stack"`
	Prune         bool       `json:"prune"`
	Items         []hashItem `json:"items"`
	SecretNames   []string   `json:"secretNames"`
}

func digestFields(fields []FieldChange, live bool) string {
	type pv struct{ P, V, O string }
	rows := make([]pv, 0, len(fields))
	for _, f := range fields {
		if live {
			rows = append(rows, pv{P: f.Path, V: f.Old})
		} else {
			rows = append(rows, pv{P: f.Path, O: f.Op, V: f.New})
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].P < rows[j].P })
	raw, _ := json.Marshal(rows)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// planHashV2 is independent of file, line and warning text. Only the HMAC
// output leaves the process, so low entropy values cannot be guessed offline.
func planHashV2(stack string, prune bool, changes []Change, secretNames []string) string {
	body := hashBody{SchemaVersion: planHashSchema, Stack: stack, Prune: prune, SecretNames: append([]string{}, secretNames...)}
	sort.Strings(body.SecretNames)
	for _, c := range changes {
		hi := hashItem{
			Key: c.Key(), Action: c.Action, Code: c.Code,
			DesiredDigest: digestFields(c.Fields, false),
			LiveDigest:    digestFields(c.Fields, true),
		}
		if isCloudKind(c.Kind) {
			hi.Cloud = cloudDigest(c)
		}
		body.Items = append(body.Items, hi)
	}
	sort.Slice(body.Items, func(i, j int) bool { return body.Items[i].Key < body.Items[j].Key })
	raw, _ := json.Marshal(body)
	mac := hmac.New(sha256.New, currentHashKey())
	mac.Write(raw)
	return HashV2Prefix + hex.EncodeToString(mac.Sum(nil))
}

// planHashV1 is the unkeyed hash of earlier releases, kept to accept old hashes.
func planHashV1(changes []Change) string {
	raw, _ := json.Marshal(changes)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:8])
}

// PlanHashV1 recomputes the v1 hash of a plan's changes.
func PlanHashV1(p Plan) string { return planHashV1(p.Changes) }

// cloudDigest covers the masked content and the verdict of a cloud item, so a
// refusal or a changed visible attribute changes the hash without raw secrets.
func cloudDigest(c Change) string {
	raw, _ := json.Marshal(struct {
		R string
		F []FieldChange
		H string `json:",omitempty"`
	}{c.Reason, c.Fields, c.CloudPlanHash})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
