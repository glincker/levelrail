package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
)

// canonical encodes without HTML escaping and with a trailing newline,
// so the same feed always yields the same signed bytes.
func canonical(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type buildOpts struct {
	Dir      string
	PrevFeed string
	Sequence int64
	Now      time.Time
	Lim      Limits
}

func buildFeed(o buildOpts) ([]byte, *Feed, error) {
	items, probs := validatePath(o.Dir, validateOpts{Now: o.Now, Lim: o.Lim})
	var errs []string
	for _, p := range probs {
		if !p.Warn {
			errs = append(errs, p.String())
		}
	}
	if len(errs) > 0 {
		return nil, nil, errors.New("validation failed:\n" + strings.Join(errs, "\n"))
	}
	seq := o.Sequence
	if o.PrevFeed != "" {
		prev, err := readFeed(o.PrevFeed)
		if err != nil {
			return nil, nil, fmt.Errorf("previous feed: %w", err)
		}
		if seq == 0 {
			seq = prev.Sequence + 1
		}
		if seq <= prev.Sequence {
			return nil, nil, fmt.Errorf("sequence %d must be greater than previous %d", seq, prev.Sequence)
		}
	}
	if seq < 1 {
		seq = 1
	}
	live := make([]Item, 0, len(items))
	for _, it := range items {
		if exp, _ := parseTime(it.ExpiresAt); exp.After(o.Now) {
			live = append(live, it)
		}
	}
	sort.SliceStable(live, func(i, j int) bool {
		if live[i].IssuedAt != live[j].IssuedAt {
			return live[i].IssuedAt < live[j].IssuedAt
		}
		return live[i].ID < live[j].ID
	})
	f := &Feed{
		Schema:      1,
		Sequence:    seq,
		GeneratedAt: o.Now.UTC().Format(time.RFC3339),
		ExpiresAt:   o.Now.Add(o.Lim.FeedMaxAge).UTC().Format(time.RFC3339),
		Items:       live,
	}
	raw, err := canonical(f)
	if err != nil {
		return nil, nil, err
	}
	if len(raw) > o.Lim.MaxBytes {
		return nil, nil, fmt.Errorf("built feed is %d bytes, cap is %d", len(raw), o.Lim.MaxBytes)
	}
	return raw, f, nil
}

// errNeedsOffline marks a feed that holds tiers the caller may not sign.
var errNeedsOffline = errors.New("feed needs offline signing")

type signOpts struct {
	KeyPaths  []string
	Existing  []SigEntry
	OnlyTiers []string
	Partial   bool
}

// signFeed signs the exact bytes with each key file and, unless Partial,
// refuses to emit a signature set that cannot satisfy every item tier.
func signFeed(raw []byte, roles *Roles, o signOpts) ([]SigEntry, error) {
	var f Feed
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("feed: %w", err)
	}
	if len(o.OnlyTiers) > 0 {
		for _, it := range f.Items {
			if !slices.Contains(o.OnlyTiers, it.Tier) {
				return nil, fmt.Errorf("%w: item %s has tier %s", errNeedsOffline, it.ID, it.Tier)
			}
		}
	}
	byID := map[string]SigEntry{}
	order := []string{}
	for _, s := range o.Existing {
		if _, ok := byID[s.KeyID]; !ok {
			order = append(order, s.KeyID)
		}
		byID[s.KeyID] = s
	}
	usedPub := map[string]string{}
	for _, p := range o.KeyPaths {
		k, priv, err := loadKeyFile(p)
		if err != nil {
			return nil, err
		}
		rk := roles.key(k.KeyID)
		if rk == nil || rk.PublicKey != k.PublicKey {
			return nil, fmt.Errorf("key %s is not in the roles file", k.KeyID)
		}
		if roles.revoked(k.KeyID, nil) {
			return nil, fmt.Errorf("key %s is revoked", k.KeyID)
		}
		if other, dup := usedPub[k.PublicKey]; dup {
			return nil, fmt.Errorf("key files %s and %s are the same key", other, p)
		}
		usedPub[k.PublicKey] = p
		if _, ok := byID[k.KeyID]; !ok {
			order = append(order, k.KeyID)
		}
		byID[k.KeyID] = SigEntry{KeyID: k.KeyID, Sig: base64.StdEncoding.EncodeToString(ed25519.Sign(priv, raw))}
	}
	out := make([]SigEntry, 0, len(order))
	for _, id := range order {
		out = append(out, byID[id])
	}
	if o.Partial {
		return out, nil
	}
	ids := make([]string, 0, len(out))
	for _, s := range out {
		ids = append(ids, s.KeyID)
	}
	for _, it := range f.Items {
		if !tierSigned(it.Tier, ids, roles) {
			return nil, fmt.Errorf("item %s (tier %s) needs %d signing keys with that tier, have %d signatures", it.ID, it.Tier, sigsNeeded[it.Tier], len(out))
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no key given")
	}
	return out, nil
}
