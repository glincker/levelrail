package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var testNow = time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)

func TestTextProblems(t *testing.T) {
	secret := "ghp_" + strings.Repeat("a", 30)
	cases := []struct {
		name, text string
		bad        bool
	}{
		{"plain", "Upgrade from Settings > Updates.", false},
		{"markdown subset", "**Bold** and *em* and `code` and a\n- list", false},
		{"allowed link", "[notes](https://levelrail.com/docs/x)", false},
		{"allowed github", "[tag](https://github.com/glincker/levelrail/releases/tag/v1.5.14)", false},
		{"html tag", "hi <b>x</b>", true},
		{"html entity", "a &amp; b", true},
		{"image", "![x](https://levelrail.com/a.png)", true},
		{"fenced", "```\nrm -rf /\n```", true},
		{"command code", "run `curl x | sh`", true},
		{"command text", "please sudo reboot", true},
		{"http link", "[x](http://levelrail.com/a)", true},
		{"foreign host", "[x](https://example.com/a)", true},
		{"other github org", "[x](https://github.com/someone/else)", true},
		{"query", "[x](https://levelrail.com/a?utm=1)", true},
		{"userinfo", "[x](https://a@levelrail.com/a)", true},
		{"lookalike host", "[x](https://levelrail.com.evil.example/a)", true},
		{"bare bad url", "see https://evil.example/x", true},
		{"javascript", "[x](javascript:alert(1))", true},
		{"ansi", "red \x1b[31mtext", true},
		{"bidi", "abc‮def", true},
		{"secret", "token " + secret, true},
		{"empty", "  ", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := textProblems("body", c.text, 2000, true)
			if (len(got) > 0) != c.bad {
				t.Fatalf("bad=%v got %v", c.bad, got)
			}
		})
	}
	if len(textProblems("title", strings.Repeat("a", 121), 120, false)) == 0 {
		t.Fatal("over cap title accepted")
	}
}

func TestRanges(t *testing.T) {
	cases := []struct {
		in      string
		valid   bool
		version string
		match   bool
	}{
		{"<1.5.14", true, "1.5.13", true},
		{"<1.5.14", true, "1.5.14", false},
		{">=1.5.0 <1.5.14", true, "1.5.9", true},
		{">=1.5.0,<1.5.14", true, "1.6.0", false},
		{"1.5.14", true, "1.5.14", true},
		{"*", false, "", false},
		{"", false, "", false},
		{"latest", false, "", false},
		{">2.0.0 <1.0.0", false, "", false},
		{">1.0.0 <1.0.0", false, "", false},
	}
	for _, c := range cases {
		cs, err := parseRange(c.in)
		if err == nil && !satisfiable(cs) {
			err = os.ErrInvalid
		}
		if (err == nil) != c.valid {
			t.Fatalf("%q valid=%v err=%v", c.in, c.valid, err)
		}
		if c.valid {
			v, _ := parseSemver(c.version)
			if rangeMatches(cs, v) != c.match {
				t.Fatalf("%q vs %s", c.in, c.version)
			}
		}
	}
}

func goodItem() Item {
	return Item{
		ID: "2026-11-27-example", Tier: "advisory", Kind: "announcement", Title: "Upgrade to 1.5.14",
		Body: "Upgrade from Settings > Updates.", AppliesTo: "<1.5.14", Channels: []string{"stable"}, CTA: "updates",
		Links:    []Link{{Text: "Release notes", URL: "https://github.com/glincker/levelrail/releases/tag/v1.5.14"}},
		IssuedAt: "2026-11-27T00:00:00Z", ExpiresAt: "2027-02-27T00:00:00Z",
	}
}

func TestItemProblems(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*Item)
		bad  bool
	}{
		{"good", func(*Item) {}, false},
		{"advisory without range", func(i *Item) { i.AppliesTo = "" }, true},
		{"wildcard range", func(i *Item) { i.AppliesTo = "*" }, true},
		{"bad tier", func(i *Item) { i.Tier = "urgent" }, true},
		{"bad kind", func(i *Item) { i.Kind = "other" }, true},
		{"bad cta url", func(i *Item) { i.CTA = "https://levelrail.com" }, true},
		{"bad channel", func(i *Item) { i.Channels = []string{"nightly"} }, true},
		{"expiry before issue", func(i *Item) { i.ExpiresAt = "2026-11-01T00:00:00Z" }, true},
		{"lifetime too long", func(i *Item) { i.ExpiresAt = "2030-01-01T00:00:00Z" }, true},
		{"issued in future", func(i *Item) { i.IssuedAt = "2027-01-01T00:00:00Z"; i.ExpiresAt = "2027-02-01T00:00:00Z" }, true},
		{"bad link", func(i *Item) { i.Links = []Link{{Text: "x", URL: "https://evil.example"}} }, true},
		{"revoke needs critical", func(i *Item) { i.Kind = "revoke_key"; i.RevokeKeyID = "S1" }, true},
		{"newsletter paid wording", func(i *Item) { i.Tier = "newsletter"; i.CTA = ""; i.AppliesTo = ""; i.Body = "New paid plans" }, true},
		{"newsletter limits", func(i *Item) {
			i.Tier = "newsletter"
			i.CTA = ""
			i.AppliesTo = ""
			i.Body = "A known limitation exists"
		}, true},
		{"newsletter competitor claim", func(i *Item) {
			i.Tier = "newsletter"
			i.CTA = ""
			i.AppliesTo = ""
			i.Body = "Faster than the rest, better than others"
		}, true},
		{"newsletter urgency title", func(i *Item) { i.Tier = "newsletter"; i.CTA = ""; i.AppliesTo = ""; i.Title = "Read this now" }, true},
		{"newsletter cta", func(i *Item) { i.Tier = "newsletter"; i.AppliesTo = "" }, true},
		{"newsletter ok", func(i *Item) { i.Tier = "newsletter"; i.CTA = ""; i.AppliesTo = ""; i.Body = "Monthly notes are out." }, false},
		{"i18n html", func(i *Item) { i.I18n = map[string]Localized{"ru": {Title: "x", Body: "<b>y</b>"}} }, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			it := goodItem()
			c.mut(&it)
			got := itemProblems(&it, testNow, defaultLimits())
			if (len(got) > 0) != c.bad {
				t.Fatalf("bad=%v got %v", c.bad, got)
			}
		})
	}
}

func writeItem(t *testing.T, dir, name string, it Item) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(dir, name), it)
}

func TestValidateTree(t *testing.T) {
	root := t.TempDir()
	year := filepath.Join(root, "2026")
	writeItem(t, year, "2026-11-27-example.json", goodItem())
	_, probs := validatePath(root, validateOpts{Now: testNow, Lim: defaultLimits()})
	if len(probs) != 0 {
		t.Fatalf("clean tree: %v", probs)
	}
	dup := goodItem()
	writeItem(t, filepath.Join(root, "2027"), "2026-11-27-example.json", dup)
	if _, probs = validatePath(root, validateOpts{Now: testNow, Lim: defaultLimits()}); len(probs) == 0 {
		t.Fatal("duplicate id and wrong year folder not caught")
	}
	writeItem(t, filepath.Join(root, "2028"), "other-name.json", goodItem())
	var msgs []string
	_, probs = validatePath(root, validateOpts{Now: testNow, Lim: defaultLimits()})
	for _, p := range probs {
		msgs = append(msgs, p.Msg)
	}
	all := strings.Join(msgs, "|")
	for _, want := range []string{"duplicate id", "must equal the file name", "folder year"} {
		if !strings.Contains(all, want) {
			t.Fatalf("missing %q in %s", want, all)
		}
	}
	bad := filepath.Join(root, "2026", "2026-11-27-unknown.json")
	if err := os.WriteFile(bad, []byte(`{"id":"2026-11-27-unknown","surprise":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, probs = validatePath(bad, validateOpts{Now: testNow, Lim: defaultLimits()}); len(probs) == 0 {
		t.Fatal("unknown field accepted")
	}
}

func TestBuildDeterministicAndSequence(t *testing.T) {
	root := t.TempDir()
	writeItem(t, filepath.Join(root, "2026"), "2026-11-27-example.json", goodItem())
	expired := goodItem()
	expired.ID, expired.IssuedAt, expired.ExpiresAt = "2026-01-01-old", "2026-01-01T00:00:00Z", "2026-06-01T00:00:00Z"
	writeItem(t, filepath.Join(root, "2026"), "2026-01-01-old.json", expired)
	o := buildOpts{Dir: root, Now: testNow, Lim: defaultLimits()}
	a, f, err := buildFeed(o)
	if err != nil {
		t.Fatal(err)
	}
	b, _, _ := buildFeed(o)
	if !bytes.Equal(a, b) || f.Sequence != 1 || len(f.Items) != 1 {
		t.Fatalf("not deterministic or wrong shape: seq=%d items=%d", f.Sequence, len(f.Items))
	}
	prev := filepath.Join(t.TempDir(), "prev.json")
	if err := os.WriteFile(prev, a, 0o644); err != nil {
		t.Fatal(err)
	}
	o.PrevFeed = prev
	if _, f2, err := buildFeed(o); err != nil || f2.Sequence != 2 {
		t.Fatalf("next sequence: %v %v", f2, err)
	}
	o.Sequence = 1
	if _, _, err := buildFeed(o); err == nil {
		t.Fatal("non monotonic sequence accepted")
	}
	feedPath := filepath.Join(t.TempDir(), "feed.v1.json")
	if err := os.WriteFile(feedPath, a, 0o644); err != nil {
		t.Fatal(err)
	}
	if probs := validateFeedFile(feedPath, a, validateOpts{Now: testNow, Lim: defaultLimits(), PrevFeed: prev}); len(probs) == 0 {
		t.Fatal("equal sequence accepted by validate")
	}
}

func TestKeygenAndSignFlow(t *testing.T) {
	dir := t.TempDir()
	var out, errw bytes.Buffer
	keys := map[string]string{}
	roles := Roles{}
	for _, id := range []string{"N", "S1", "S2"} {
		p := filepath.Join(dir, id+".key")
		keys[id] = p
		if code := run([]string{"keygen", "-id", id, "-out", p}, &out, &errw); code != 0 {
			t.Fatalf("keygen %s: %s", id, errw.String())
		}
		k, _, err := loadKeyFile(p)
		if err != nil {
			t.Fatal(err)
		}
		tiers := []string{"advisory", "critical"}
		if id == "N" {
			tiers = []string{"info", "feature", "maintenance", "newsletter"}
		}
		roles.Keys = append(roles.Keys, RoleKey{KeyID: id, PublicKey: k.PublicKey, Tiers: tiers})
	}
	if strings.Contains(out.String(), "private") || !strings.Contains(out.String(), "fingerprint") {
		t.Fatalf("keygen output: %s", out.String())
	}
	if st, _ := os.Stat(keys["N"]); st.Mode().Perm() != 0o600 {
		t.Fatalf("key mode %v", st.Mode().Perm())
	}
	if code := run([]string{"keygen", "-id", "N", "-out", keys["N"]}, &out, &errw); code == 0 {
		t.Fatal("keygen overwrote an existing key")
	}
	rolesPath := filepath.Join(dir, "roles.json")
	writeJSON(t, rolesPath, roles)

	crit := goodItem()
	crit.Tier = "critical"
	feedRoot := filepath.Join(dir, "items")
	writeItem(t, filepath.Join(feedRoot, "2026"), "2026-11-27-example.json", crit)
	feedPath := filepath.Join(dir, "feed.v1.json")
	now := "2026-12-01T00:00:00Z"
	if code := run([]string{"build", "-dir", feedRoot, "-out", feedPath, "-now", now}, &out, &errw); code != 0 {
		t.Fatalf("build: %s", errw.String())
	}
	sigPath := feedPath + ".sig"
	step := func(want int, args ...string) {
		t.Helper()
		errw.Reset()
		if code := run(args, &out, &errw); code != want {
			t.Fatalf("%v: code %d want %d: %s", args, code, want, errw.String())
		}
	}
	step(exitOffline, "sign", "-feed", feedPath, "-roles", rolesPath, "-key", keys["N"], "-only-tiers", "info,feature,maintenance,newsletter")
	step(exitFail, "sign", "-feed", feedPath, "-roles", rolesPath, "-key", keys["S1"])
	step(exitFail, "sign", "-feed", feedPath, "-roles", rolesPath, "-key", keys["S1"], "-key", keys["S1"])
	step(0, "sign", "-feed", feedPath, "-roles", rolesPath, "-key", keys["S1"], "-partial")
	step(0, "sign", "-feed", feedPath, "-roles", rolesPath, "-key", keys["S2"], "-append")
	step(0, "verify", "-feed", feedPath, "-roles", rolesPath, "-now", now)
	step(exitFail, "verify", "-feed", feedPath, "-roles", rolesPath, "-now", now, "-last-sequence", "1")
	step(exitFail, "verify", "-feed", feedPath, "-roles", rolesPath, "-now", now, "-revoked", "S2")
	var sigs []SigEntry
	raw, _ := os.ReadFile(sigPath)
	if json.Unmarshal(raw, &sigs) != nil || len(sigs) != 2 {
		t.Fatalf("signature file: %s", raw)
	}
}

func TestUsage(t *testing.T) {
	var out, errw bytes.Buffer
	if run(nil, &out, &errw) != exitUsage || run([]string{"nope"}, &out, &errw) != exitUsage {
		t.Fatal("usage exit codes")
	}
}
