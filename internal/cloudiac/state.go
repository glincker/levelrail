package cloudiac

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// StatePurpose names the master derived key that wraps every per record state key.
const StatePurpose = "cloud-state"

// State errors. Messages never carry state bytes or key material.
var (
	ErrStateConfig         = errors.New("cloudiac: state store is not configured")
	ErrStateKeyUnavailable = errors.New("cloudiac: state key is unavailable, the master key is not loaded")
	ErrStateTooLarge       = errors.New("cloudiac: state exceeds the size cap")
	ErrStateTampered       = errors.New("cloudiac: sealed state failed authentication")
)

// Sealed layout: magic, then the record key sealed under the derived key, then the state sealed under the record key.
// Both layers bind the account and stack as AAD, so a blob moved to another row fails to open.
var stateMagic = []byte("lrcs1")

const (
	recordKeyLen = 32
	gcmNonceLen  = 12
)

// KeyDeriver is the part of secrets.Manager the state store needs.
type KeyDeriver interface {
	DeriveKey(purpose string) []byte
}

// StateDB is the part of the store the state store needs.
type StateDB interface {
	GetCloudState(ctx context.Context, account, stack string) (store.CloudState, error)
	PutCloudState(ctx context.Context, lock store.CloudStateLock, expectVersion int64, sealed []byte, now time.Time) (int64, error)
	PutCloudPlan(ctx context.Context, p store.CloudPlan) error
	GetCloudPlan(ctx context.Context, account, stack string) (store.CloudPlan, error)
	DeleteCloudPlan(ctx context.Context, account, stack, hash string) error
	InsertCloudStateQuarantine(ctx context.Context, q store.CloudStateQuarantine) (int64, error)
	ListCloudStateQuarantine(ctx context.Context, account, stack string) ([]store.CloudStateQuarantine, error)
}

// StateStore seals state before it reaches the DB. It never logs and has no method that serves state over an API.
type StateStore struct {
	db       StateDB
	keys     KeyDeriver
	maxBytes int64
	now      func() time.Time
}

// NewStateStore needs the size cap from runtime settings; zero refuses.
func NewStateStore(db StateDB, keys KeyDeriver, maxBytes int64) (*StateStore, error) {
	if db == nil || keys == nil || maxBytes <= 0 {
		return nil, ErrStateConfig
	}
	return &StateStore{db: db, keys: keys, maxBytes: maxBytes, now: time.Now}, nil
}

// Load returns the plaintext state and its version. A stack with no state yet returns nil and version 0.
func (s *StateStore) Load(ctx context.Context, account, stack string) ([]byte, int64, error) {
	rec, err := s.db.GetCloudState(ctx, account, stack)
	if errors.Is(err, store.ErrCloudStateNotFound) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	kek, err := s.kek()
	if err != nil {
		return nil, 0, err
	}
	pt, err := openLayers(kek, "key", "state", account, stack, rec.Sealed)
	if err != nil {
		return nil, 0, err
	}
	if int64(len(pt)) > s.maxBytes {
		return nil, 0, ErrStateTooLarge
	}
	return pt, rec.Version, nil
}

// Save seals and writes state under a live lease. expectVersion is the version Load returned.
func (s *StateStore) Save(ctx context.Context, lock store.CloudStateLock, expectVersion int64, state []byte) (int64, error) {
	if int64(len(state)) > s.maxBytes {
		return 0, ErrStateTooLarge
	}
	kek, err := s.kek()
	if err != nil {
		return 0, err
	}
	blob, err := sealLayers(kek, "key", "state", lock.Account, lock.Stack, state)
	if err != nil {
		return 0, err
	}
	return s.db.PutCloudState(ctx, lock, expectVersion, blob, s.now())
}

// Quarantine seals state that could not be stored live and returns the row id. It takes no lease and no
// size cap, since the blob describes real resources and losing it would orphan them.
func (s *StateStore) Quarantine(ctx context.Context, account, stack string, fence int64, reason string, state []byte) (int64, error) {
	kek, err := s.kek()
	if err != nil {
		return 0, err
	}
	blob, err := sealLayers(kek, "quarantine-key", "quarantine", account, stack, state)
	if err != nil {
		return 0, err
	}
	return s.db.InsertCloudStateQuarantine(ctx, store.CloudStateQuarantine{
		Account: account, Stack: stack, Sealed: blob, Fence: fence, Reason: reason, CreatedAt: s.now()})
}

// Quarantined lists the unresolved quarantine rows of a stack; any row means the stack must not run.
func (s *StateStore) Quarantined(ctx context.Context, account, stack string) ([]store.CloudStateQuarantine, error) {
	return s.db.ListCloudStateQuarantine(ctx, account, stack)
}

// OpenQuarantine returns the plaintext of a quarantine row for an operator recovery step.
func (s *StateStore) OpenQuarantine(q store.CloudStateQuarantine) ([]byte, error) {
	kek, err := s.kek()
	if err != nil {
		return nil, err
	}
	return openLayers(kek, "quarantine-key", "quarantine", q.Account, q.Stack, q.Sealed)
}

func (s *StateStore) kek() ([]byte, error) {
	k := s.keys.DeriveKey(StatePurpose)
	if len(k) != recordKeyLen {
		return nil, ErrStateKeyUnavailable
	}
	return k, nil
}

func stateAAD(layer, account, stack string) []byte {
	return []byte("levelrail-cloud-state-v1\x00" + layer + "\x00" + account + "\x00" + stack)
}

// sealLayers seals under distinct AAD layers per blob kind, so a sealed plan can never open as state.
func sealLayers(kek []byte, keyLayer, bodyLayer, account, stack string, plaintext []byte) ([]byte, error) {
	rk := make([]byte, recordKeyLen)
	if _, err := rand.Read(rk); err != nil {
		return nil, err
	}
	defer clear(rk)
	wrapped, err := gcmSeal(kek, rk, stateAAD(keyLayer, account, stack))
	if err != nil {
		return nil, err
	}
	body, err := gcmSeal(rk, plaintext, stateAAD(bodyLayer, account, stack))
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(stateMagic)+2+len(wrapped)+len(body))
	out = append(out, stateMagic...)
	out = binary.BigEndian.AppendUint16(out, uint16(len(wrapped))) //nolint:gosec // wrapped key is 60 bytes
	out = append(out, wrapped...)
	return append(out, body...), nil
}

func openLayers(kek []byte, keyLayer, bodyLayer, account, stack string, blob []byte) ([]byte, error) {
	if len(blob) < len(stateMagic)+2 || string(blob[:len(stateMagic)]) != string(stateMagic) {
		return nil, ErrStateTampered
	}
	rest := blob[len(stateMagic):]
	n := int(binary.BigEndian.Uint16(rest))
	rest = rest[2:]
	if n > len(rest) {
		return nil, ErrStateTampered
	}
	rk, err := gcmOpen(kek, rest[:n], stateAAD(keyLayer, account, stack))
	if err != nil || len(rk) != recordKeyLen {
		return nil, ErrStateTampered
	}
	defer clear(rk)
	pt, err := gcmOpen(rk, rest[n:], stateAAD(bodyLayer, account, stack))
	if err != nil {
		return nil, ErrStateTampered
	}
	return pt, nil
}

func gcmSeal(key, plaintext, aad []byte) ([]byte, error) {
	g, err := newStateGCM(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcmNonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return g.Seal(nonce, nonce, plaintext, aad), nil
}

func gcmOpen(key, sealed, aad []byte) ([]byte, error) {
	g, err := newStateGCM(key)
	if err != nil {
		return nil, err
	}
	if len(sealed) < gcmNonceLen+g.Overhead() {
		return nil, ErrStateTampered
	}
	return g.Open(nil, sealed[:gcmNonceLen], sealed[gcmNonceLen:], aad)
}

func newStateGCM(key []byte) (cipher.AEAD, error) {
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("cloudiac: state cipher: %w", err)
	}
	return cipher.NewGCM(b)
}
