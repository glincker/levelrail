package cloudiac

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// ErrPlanNotFound means the stack has no saved plan to apply.
var ErrPlanNotFound = errors.New("cloudiac: no saved plan for this stack")

// SavedPlan is the reviewed plan of one stack. Changes are kept beside the bytes, sealed with them,
// so policy and cost are re-checked before any step runs.
type SavedPlan struct {
	Hash         string           `json:"-"`
	Plan         []byte           `json:"plan"`
	Changes      []ResourceChange `json:"changes"`
	StateVersion int64            `json:"state_version"`
	Destroy      bool             `json:"destroy"`
	CreatedAt    time.Time        `json:"-"`
}

// PlanHash is the digest a reviewer approves; apply refuses any other.
func PlanHash(plan []byte) string {
	sum := sha256.Sum256(plan)
	return hex.EncodeToString(sum[:])
}

// SavePlan seals the plan and replaces the stack's saved plan.
func (s *StateStore) SavePlan(ctx context.Context, account, stack string, p SavedPlan) (string, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	defer clear(body)
	if int64(len(body)) > s.maxBytes {
		return "", ErrStateTooLarge
	}
	kek, err := s.kek()
	if err != nil {
		return "", err
	}
	blob, err := sealLayers(kek, "plan-key", "plan", account, stack, body)
	if err != nil {
		return "", err
	}
	hash := PlanHash(p.Plan)
	return hash, s.db.PutCloudPlan(ctx, store.CloudPlan{Account: account, Stack: stack, Hash: hash, Sealed: blob, CreatedAt: s.now()})
}

// LoadPlan opens the saved plan and checks its bytes still match the recorded hash.
func (s *StateStore) LoadPlan(ctx context.Context, account, stack string) (SavedPlan, error) {
	rec, err := s.db.GetCloudPlan(ctx, account, stack)
	if errors.Is(err, store.ErrCloudPlanNotFound) {
		return SavedPlan{}, ErrPlanNotFound
	}
	if err != nil {
		return SavedPlan{}, err
	}
	kek, err := s.kek()
	if err != nil {
		return SavedPlan{}, err
	}
	pt, err := openLayers(kek, "plan-key", "plan", account, stack, rec.Sealed)
	if err != nil {
		return SavedPlan{}, err
	}
	defer clear(pt)
	var p SavedPlan
	if err := json.Unmarshal(pt, &p); err != nil || PlanHash(p.Plan) != rec.Hash {
		return SavedPlan{}, ErrStateTampered
	}
	p.Hash, p.CreatedAt = rec.Hash, rec.CreatedAt
	return p, nil
}

// DropPlan removes the saved plan if it is still the one with this hash.
func (s *StateStore) DropPlan(ctx context.Context, account, stack, hash string) error {
	return s.db.DeleteCloudPlan(ctx, account, stack, hash)
}
