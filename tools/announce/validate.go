package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	idRe       = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{2,79}$`)
	langRe     = regexp.MustCompile(`^[a-z]{2,3}(-[A-Za-z0-9]{2,8})?$`)
	keyIDRe    = regexp.MustCompile(`^[A-Za-z0-9_-]{1,16}$`)
	flagRe     = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{0,63}$`)
	validTiers = map[string]bool{"critical": true, "advisory": true, "feature": true, "maintenance": true, "info": true, "newsletter": true}
	validKinds = map[string]bool{"announcement": true, "revoke_key": true}
	validChans = map[string]bool{"stable": true, "beta": true, "edge": true}
	validCTAs  = map[string]bool{"updates": true, "settings": true}
)

// Problem is one validation finding. Warn findings never fail the run.
type Problem struct {
	File string
	ID   string
	Msg  string
	Warn bool
}

func (p Problem) String() string {
	lvl := "error"
	if p.Warn {
		lvl = "warn"
	}
	return fmt.Sprintf("%s: %s: %s: %s", lvl, p.File, p.ID, p.Msg)
}

func parseTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}

// itemProblems holds every rule that does not depend on the file path,
// so the instance side verifier applies the same set.
func itemProblems(it *Item, now time.Time, lim Limits) []string {
	var out []string
	add := func(f string, a ...any) { out = append(out, fmt.Sprintf(f, a...)) }
	if !idRe.MatchString(it.ID) {
		add("id must match %s", idRe)
	}
	if !validTiers[it.Tier] {
		add("unknown tier %q", it.Tier)
	}
	if !validKinds[it.Kind] {
		add("unknown kind %q", it.Kind)
	}
	if it.Kind == "revoke_key" {
		if it.Tier != "critical" {
			add("revoke_key items must be tier critical")
		}
		if !keyIDRe.MatchString(it.RevokeKeyID) {
			add("revoke_key needs a valid revoke_key_id")
		}
	} else if it.RevokeKeyID != "" {
		add("revoke_key_id is only for kind revoke_key")
	}
	out = append(out, textProblems("title", it.Title, lim.TitleRunes, false)...)
	out = append(out, textProblems("body", it.Body, lim.BodyRunes, true)...)
	for lang, l := range it.I18n {
		if !langRe.MatchString(lang) {
			add("i18n key %q is not a language tag", lang)
		}
		out = append(out, textProblems("i18n."+lang+".title", l.Title, lim.TitleRunes, false)...)
		out = append(out, textProblems("i18n."+lang+".body", l.Body, lim.BodyRunes, true)...)
	}
	out = append(out, wordingProblems(it)...)
	if len(it.Links) > 5 {
		add("at most 5 links")
	}
	for i, l := range it.Links {
		out = append(out, textProblems(fmt.Sprintf("links[%d].text", i), l.Text, 60, false)...)
		if p := linkProblem(l.URL); p != "" {
			add("links[%d]: %s", i, p)
		}
	}
	for _, c := range it.Channels {
		if !validChans[c] {
			add("unknown channel %q", c)
		}
	}
	for _, f := range it.RequiresFlags {
		if !flagRe.MatchString(f) {
			add("bad requires_flags entry %q", f)
		}
	}
	if it.CTA != "" && !validCTAs[it.CTA] {
		add("cta must be one of updates, settings")
	}
	if it.CTA != "" && it.Tier == "newsletter" {
		add("newsletter items cannot carry a cta")
	}
	switch {
	case it.AppliesTo == "" && (it.Tier == "critical" || it.Tier == "advisory"):
		add("applies_to is required for tier %s", it.Tier)
	case it.AppliesTo != "":
		cs, err := parseRange(it.AppliesTo)
		if err != nil {
			add("applies_to: %v", err)
		} else if !satisfiable(cs) {
			add("applies_to %q matches no version", it.AppliesTo)
		}
	}
	return append(out, timeProblems(it, now, lim)...)
}

func timeProblems(it *Item, now time.Time, lim Limits) []string {
	var out []string
	iss, e1 := parseTime(it.IssuedAt)
	exp, e2 := parseTime(it.ExpiresAt)
	if e1 != nil {
		out = append(out, "issued_at must be RFC 3339")
	}
	if e2 != nil {
		out = append(out, "expires_at must be RFC 3339")
	}
	if e1 != nil || e2 != nil {
		return out
	}
	switch {
	case !exp.After(iss):
		out = append(out, "expires_at must be after issued_at")
	case exp.Sub(iss) > lim.MaxLifetime:
		out = append(out, fmt.Sprintf("lifetime exceeds %d days", int(lim.MaxLifetime.Hours()/24)))
	}
	if iss.After(now.Add(lim.Skew)) {
		out = append(out, "issued_at is in the future")
	}
	return out
}

type validateOpts struct {
	Now      time.Time
	Lim      Limits
	PrevFeed string
}

// loadItem decodes an authoring file strictly: unknown fields are errors
// here even though instances ignore them.
func loadItem(path string) (*Item, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var it Item
	if err := dec.Decode(&it); err != nil {
		return nil, fmt.Errorf("bad json: %w", err)
	}
	if dec.More() {
		return nil, fmt.Errorf("bad json: trailing data")
	}
	return &it, nil
}

func collectItemFiles(root string) ([]string, error) {
	st, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		return []string{root}, nil
	}
	var files []string
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() && (d.Name() == "schema" || d.Name() == "keys" || d.Name() == "dist") {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".json") {
			files = append(files, p)
		}
		return nil
	})
	sort.Strings(files)
	return files, err
}

func validatePath(root string, o validateOpts) ([]Item, []Problem) {
	files, err := collectItemFiles(root)
	if err != nil {
		return nil, []Problem{{File: root, Msg: err.Error()}}
	}
	var items []Item
	var probs []Problem
	seen := map[string]string{}
	for _, f := range files {
		if raw, rerr := os.ReadFile(f); rerr == nil && bytes.Contains(raw, []byte(`"sequence"`)) && bytes.Contains(raw, []byte(`"items"`)) {
			probs = append(probs, validateFeedFile(f, raw, o)...)
			continue
		}
		it, lerr := loadItem(f)
		if lerr != nil {
			probs = append(probs, Problem{File: f, Msg: lerr.Error()})
			continue
		}
		for _, m := range itemProblems(it, o.Now, o.Lim) {
			probs = append(probs, Problem{File: f, ID: it.ID, Msg: m})
		}
		if base := strings.TrimSuffix(filepath.Base(f), ".json"); base != it.ID {
			probs = append(probs, Problem{File: f, ID: it.ID, Msg: "id must equal the file name"})
		}
		if year := filepath.Base(filepath.Dir(f)); len(year) == 4 && len(it.IssuedAt) >= 4 && year != it.IssuedAt[:4] {
			probs = append(probs, Problem{File: f, ID: it.ID, Msg: "folder year must match issued_at year"})
		}
		if prev, dup := seen[it.ID]; dup {
			probs = append(probs, Problem{File: f, ID: it.ID, Msg: "duplicate id, also in " + prev})
		}
		seen[it.ID] = f
		if exp, e := parseTime(it.ExpiresAt); e == nil && !exp.After(o.Now) {
			probs = append(probs, Problem{File: f, ID: it.ID, Msg: "already expired, build will skip it", Warn: true})
		}
		items = append(items, *it)
	}
	if len(items) > o.Lim.MaxItems {
		probs = append(probs, Problem{File: root, Msg: fmt.Sprintf("%d items exceed cap %d", len(items), o.Lim.MaxItems)})
	}
	return items, probs
}

// validateFeedFile checks an already built feed, including that its
// sequence moves past the previous feed.
func validateFeedFile(path string, raw []byte, o validateOpts) []Problem {
	var probs []Problem
	fail := func(id, m string) { probs = append(probs, Problem{File: path, ID: id, Msg: m}) }
	if len(raw) > o.Lim.MaxBytes {
		fail("", fmt.Sprintf("feed is %d bytes, cap is %d", len(raw), o.Lim.MaxBytes))
	}
	var f Feed
	if err := json.Unmarshal(raw, &f); err != nil {
		return append(probs, Problem{File: path, Msg: "bad json: " + err.Error()})
	}
	if f.Schema != 1 {
		fail("", "schema must be 1")
	}
	if f.Sequence < 1 {
		fail("", "sequence must be at least 1")
	}
	if o.PrevFeed != "" {
		prev, err := readFeed(o.PrevFeed)
		switch {
		case err != nil:
			fail("", "previous feed: "+err.Error())
		case f.Sequence <= prev.Sequence:
			fail("", fmt.Sprintf("sequence %d must be greater than previous %d", f.Sequence, prev.Sequence))
		}
	}
	if exp, err := parseTime(f.ExpiresAt); err != nil {
		fail("", "feed expires_at must be RFC 3339")
	} else if gen, gerr := parseTime(f.GeneratedAt); gerr != nil || !exp.After(gen) {
		fail("", "feed generated_at and expires_at are inconsistent")
	}
	if len(f.Items) > o.Lim.MaxItems {
		fail("", fmt.Sprintf("%d items exceed cap %d", len(f.Items), o.Lim.MaxItems))
	}
	ids := map[string]bool{}
	for i := range f.Items {
		it := &f.Items[i]
		for _, m := range itemProblems(it, o.Now, o.Lim) {
			fail(it.ID, m)
		}
		if ids[it.ID] {
			fail(it.ID, "duplicate id")
		}
		ids[it.ID] = true
	}
	return probs
}

func readFeed(path string) (*Feed, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f Feed
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, err
	}
	return &f, nil
}
