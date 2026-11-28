package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"slices"
	"time"
)

// Error codes of a rejected feed. Instances use the same strings.
const (
	errTooLarge  = "too_large"
	errBadJSON   = "bad_json"
	errBadSchema = "bad_schema"
	errBadSig    = "bad_signature"
	errNoTrusted = "no_trusted_signature"
	errReplay    = "replay"
	errExpired   = "feed_expired"
)

// Reasons an item is dropped while the rest of the feed is kept.
const (
	dropTier     = "unknown_tier"
	dropKind     = "unknown_kind"
	dropUnsigned = "tier_not_signed"
	dropExpired  = "expired"
	dropDup      = "duplicate_id"
	dropContent  = "content"
)

type Drop struct {
	ID     string `json:"id"`
	Code   string `json:"code"`
	Detail string `json:"detail,omitempty"`
}

type VerifyOpts struct {
	Now          time.Time
	LastSequence int64
	Lim          Limits
	Revoked      map[string]bool
}

type VerifyResult struct {
	OK       bool     `json:"ok"`
	ErrCode  string   `json:"error_code,omitempty"`
	Err      string   `json:"error,omitempty"`
	Sequence int64    `json:"sequence,omitempty"`
	Signers  []string `json:"signers,omitempty"`
	Accepted []string `json:"accepted"`
	Dropped  []Drop   `json:"dropped"`
	Revoke   []string `json:"revoke_keys,omitempty"`
}

func reject(code, msg string) VerifyResult {
	return VerifyResult{ErrCode: code, Err: msg, Accepted: []string{}, Dropped: []Drop{}}
}

// verifyFeed applies the exact checks an instance runs, in the same order:
// size, signatures, schema, replay, feed expiry, then per item rules.
func verifyFeed(raw, sigRaw []byte, roles *Roles, o VerifyOpts) VerifyResult {
	if len(raw) > o.Lim.MaxBytes || len(sigRaw) > o.Lim.MaxBytes {
		return reject(errTooLarge, fmt.Sprintf("feed exceeds %d bytes", o.Lim.MaxBytes))
	}
	var sigs []SigEntry
	if err := json.Unmarshal(sigRaw, &sigs); err != nil {
		return reject(errBadSig, "signature file is not a json list")
	}
	signers, res := checkSignatures(raw, sigs, roles, o)
	if res != nil {
		return *res
	}
	var f Feed
	if err := json.Unmarshal(raw, &f); err != nil {
		return reject(errBadJSON, err.Error())
	}
	if f.Schema != 1 {
		return reject(errBadSchema, fmt.Sprintf("unsupported schema %d", f.Schema))
	}
	if f.Sequence <= o.LastSequence {
		return reject(errReplay, fmt.Sprintf("sequence %d is not above stored %d", f.Sequence, o.LastSequence))
	}
	if exp, err := parseTime(f.ExpiresAt); err != nil || !exp.Add(o.Lim.Skew).After(o.Now) {
		return reject(errExpired, "feed has expired")
	}
	res2 := VerifyResult{OK: true, Sequence: f.Sequence, Signers: signers, Accepted: []string{}, Dropped: []Drop{}}
	if len(f.Items) > o.Lim.MaxItems {
		f.Items = f.Items[:o.Lim.MaxItems]
	}
	seen := map[string]bool{}
	for i := range f.Items {
		it := &f.Items[i]
		if d := itemDrop(it, signers, roles, seen, o); d != nil {
			res2.Dropped = append(res2.Dropped, *d)
			continue
		}
		seen[it.ID] = true
		res2.Accepted = append(res2.Accepted, it.ID)
		if it.Kind == "revoke_key" {
			res2.Revoke = append(res2.Revoke, it.RevokeKeyID)
		}
	}
	return res2
}

// checkSignatures counts valid signatures from known, unrevoked keys.
// Unknown key ids are skipped so a rotation can sign with old and new keys;
// a bad signature from a known key rejects the whole feed.
func checkSignatures(raw []byte, sigs []SigEntry, roles *Roles, o VerifyOpts) ([]string, *VerifyResult) {
	var signers []string
	for _, s := range sigs {
		k := roles.key(s.KeyID)
		if k == nil || roles.revoked(s.KeyID, o.Revoked) {
			continue
		}
		pub, _ := base64.StdEncoding.DecodeString(k.PublicKey)
		sig, err := base64.StdEncoding.DecodeString(s.Sig)
		if err != nil || len(sig) != ed25519.SignatureSize || !ed25519.Verify(pub, raw, sig) {
			r := reject(errBadSig, "signature from key "+s.KeyID+" does not verify")
			return nil, &r
		}
		if !slices.Contains(signers, s.KeyID) {
			signers = append(signers, s.KeyID)
		}
	}
	if len(signers) == 0 {
		r := reject(errNoTrusted, "no signature from a trusted key")
		return nil, &r
	}
	return signers, nil
}

func tierSigned(tier string, signers []string, roles *Roles) bool {
	n := 0
	for _, id := range signers {
		if k := roles.key(id); k != nil && slices.Contains(k.Tiers, tier) {
			n++
		}
	}
	return n >= sigsNeeded[tier]
}

func itemDrop(it *Item, signers []string, roles *Roles, seen map[string]bool, o VerifyOpts) *Drop {
	switch {
	case !validTiers[it.Tier]:
		return &Drop{ID: it.ID, Code: dropTier}
	case !validKinds[it.Kind]:
		return &Drop{ID: it.ID, Code: dropKind}
	case seen[it.ID]:
		return &Drop{ID: it.ID, Code: dropDup}
	case !tierSigned(it.Tier, signers, roles):
		return &Drop{ID: it.ID, Code: dropUnsigned}
	}
	if exp, err := parseTime(it.ExpiresAt); err != nil || !exp.Add(o.Lim.Skew).After(o.Now) {
		return &Drop{ID: it.ID, Code: dropExpired}
	}
	if probs := itemProblems(it, o.Now, o.Lim); len(probs) > 0 {
		return &Drop{ID: it.ID, Code: dropContent, Detail: probs[0]}
	}
	return nil
}
