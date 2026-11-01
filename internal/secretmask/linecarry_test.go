package secretmask_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/secretmask"
)

func joinText(ls []secretmask.Line) string {
	var b strings.Builder
	for _, l := range ls {
		b.WriteString(l.Text + "|")
	}
	return b.String()
}

func TestLineCarryMasksSecretSplitAcrossLines(t *testing.T) {
	r := secretmask.New()
	r.Add("PART-ONE-SECRET\nPART-TWO-SECRET")
	c := secretmask.NewLineCarryFor(r)
	var got []secretmask.Line
	for _, l := range []string{"start", "key=PART-ONE-SECRET", "PART-TWO-SECRET tail", "end"} {
		got = append(got, c.Push("stdout", l)...)
	}
	got = append(got, c.Flush()...)
	out := joinText(got)
	if strings.Contains(out, "PART-ONE") || strings.Contains(out, "PART-TWO") {
		t.Fatalf("leak: %q", out)
	}
	if !strings.Contains(out, "start|") || !strings.Contains(out, "key="+secretmask.Marker+" tail|") || !strings.Contains(out, "end|") {
		t.Fatalf("surrounding text lost: %q", out)
	}
	if c.Pending() {
		t.Fatal("flush left lines held")
	}
}

func TestLineCarryHoldsTailUntilFlush(t *testing.T) {
	r := secretmask.New()
	r.Add("abcdefghij\n0123456789")
	c := secretmask.NewLineCarryFor(r)
	if out := c.Push("stdout", "abcdefghij"); len(out) != 0 || !c.Pending() {
		t.Fatalf("a possible secret prefix must be held, got %v", out)
	}
	out := c.Flush()
	if len(out) != 1 || out[0].Text != "abcdefghij" {
		t.Fatalf("idle flush must emit the held text unchanged when no secret completed: %v", out)
	}
}

func TestLineCarryNoSecretsPassesThrough(t *testing.T) {
	c := secretmask.NewLineCarryFor(secretmask.New())
	if out := c.Push("stderr", "plain"); len(out) != 1 || out[0].Text != "plain" || out[0].Stream != "stderr" {
		t.Fatalf("got %v", out)
	}
}

func TestLineCarryMasksPEMWithTrailingNewline(t *testing.T) {
	r := secretmask.New()
	r.Add("-----BEGIN KEY-----\nAAAABBBBCCCC\n-----END KEY-----\n")
	c := secretmask.NewLineCarryFor(r)
	var got []secretmask.Line
	for _, l := range []string{"before", "-----BEGIN KEY-----", "AAAABBBBCCCC", "-----END KEY-----", "after"} {
		got = append(got, c.Push("stdout", l)...)
	}
	got = append(got, c.Flush()...)
	out := joinText(got)
	if strings.Contains(out, "AAAABBBB") || strings.Contains(out, "BEGIN KEY") {
		t.Fatalf("leak: %q", out)
	}
	if !strings.Contains(out, "before|") || !strings.Contains(out, "after|") {
		t.Fatalf("surrounding text lost: %q", out)
	}
}

func TestLineCarryMasksCRLFSecretAfterScannerStripsCR(t *testing.T) {
	r := secretmask.New()
	r.Add("FIRST-LINE-SECRET\r\nSECOND-LINE-SECRET")
	c := secretmask.NewLineCarryFor(r)
	var got []secretmask.Line
	for _, l := range []string{"head", "FIRST-LINE-SECRET", "SECOND-LINE-SECRET", "tail"} {
		got = append(got, c.Push("stdout", l)...)
	}
	got = append(got, c.Flush()...)
	out := joinText(got)
	if strings.Contains(out, "FIRST-LINE") || strings.Contains(out, "SECOND-LINE") {
		t.Fatalf("leak: %q", out)
	}
	if !strings.Contains(out, "head|") || !strings.Contains(out, "tail|") {
		t.Fatalf("surrounding text lost: %q", out)
	}
}

func TestLineCarrySingleLineSecretsPassThroughImmediately(t *testing.T) {
	r := secretmask.New()
	r.Add("sk-live-abcdef123456")
	c := secretmask.NewLineCarryFor(r)
	got := c.Push("stdout", "k=sk-live-abcdef123456 end")
	if len(got) != 1 || got[0].Text != "k="+secretmask.Marker+" end" || c.Pending() {
		t.Fatalf("got %+v pending=%v", got, c.Pending())
	}
}

func TestLineCarryHoldShrinksToLongestMultiLineSecret(t *testing.T) {
	r := secretmask.New()
	r.Add(strings.Repeat("long-single-line-", 20))
	r.Add("AAAA-first-half\nBBBB-second-half")
	c := secretmask.NewLineCarryFor(r)
	var out []secretmask.Line
	for _, l := range []string{"one", "x=AAAA-first-half", "BBBB-second-half y", "two"} {
		out = append(out, c.Push("stdout", l)...)
	}
	out = append(out, c.Flush()...)
	s := joinText(out)
	if strings.Contains(s, "AAAA") || strings.Contains(s, "BBBB") || !strings.Contains(s, "one|") || !strings.Contains(s, "x="+secretmask.Marker+" y|") || !strings.Contains(s, "two|") {
		t.Fatalf("bad output %q", s)
	}
}

func BenchmarkLineCarryManyTokensOnePEM(b *testing.B) {
	r := secretmask.New()
	for i := 0; i < 500; i++ {
		r.Add(fmt.Sprintf("token-%04d-abcdefghijklmnop", i))
	}
	r.Add("-----BEGIN KEY-----\n" + strings.Repeat("QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVo=\n", 100) + "-----END KEY-----")
	line := "2026-11-01 INFO request served path=/health user=someone status=200"
	b.Run("carry", func(b *testing.B) {
		c := secretmask.NewLineCarryFor(r)
		for i := 0; i < b.N; i++ {
			c.Push("stdout", line)
		}
	})
	b.Run("perline", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			r.Mask(line)
		}
	})
}
