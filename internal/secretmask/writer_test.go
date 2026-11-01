package secretmask_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/secretmask"
)

func TestWriterMasksSecretSplitAcrossWrites(t *testing.T) {
	r := secretmask.New()
	secret := "sk-live-abcdef123456"
	r.Add(secret)
	var out bytes.Buffer
	w := secretmask.NewWriterFor(&out, r)
	for _, c := range []string{"token=sk-li", "ve-abc", "def123", "456 done"} {
		_, _ = w.Write([]byte(c))
	}
	_ = w.Close()
	got := out.String()
	if strings.Contains(got, "abcdef") || strings.Contains(got, "sk-li") {
		t.Fatalf("leak: %q", got)
	}
	if got != "token="+secretmask.Marker+" done" {
		t.Fatalf("surrounding text lost: %q", got)
	}
}

func TestWriterSplitEveryByte(t *testing.T) {
	r := secretmask.New()
	r.Add("hunter2-hunter2")
	var out bytes.Buffer
	w := secretmask.NewWriterFor(&out, r)
	for _, b := range []byte("a hunter2-hunter2 b") {
		_, _ = w.Write([]byte{b})
	}
	_ = w.Close()
	if out.String() != "a "+secretmask.Marker+" b" {
		t.Fatalf("got %q", out.String())
	}
}

func TestMaskJSONEscapedForms(t *testing.T) {
	r := secretmask.New()
	value := "pa\"ss\\word<>é1234"
	r.Add(value)
	for name, in := range map[string]string{
		"json":  `{"k":"pa\"ss\\word<>é1234","x":1}`,
		"html":  `{"k":"pa\"ss\\word` + "\\u003c\\u003e" + `é1234","x":1}`,
		"ascii": `{"k":"pa\"ss\\word<>` + "\\u00e9" + `1234","x":1}`,
	} {
		got := r.Mask(in)
		if strings.Contains(got, "word") || !strings.HasSuffix(got, `","x":1}`) || !strings.HasPrefix(got, `{"k":"`) {
			t.Errorf("%s: %q", name, got)
		}
	}
}

func TestMaskOverlappingSecretsLeaveNoTail(t *testing.T) {
	r := secretmask.New()
	r.Add("abcdefgh1234")
	r.Add("efgh1234zzzz")
	r.Add("abcdefgh")
	got := r.Mask("x abcdefgh1234zzzz y")
	if got != "x "+secretmask.Marker+" y" {
		t.Fatalf("got %q", got)
	}
}

func BenchmarkMaskNoHit(b *testing.B) {
	r := secretmask.New()
	for i := 0; i < 200; i++ {
		r.Add(strings.Repeat("s", 8) + string(rune('a'+i%26)) + strings.Repeat("k", i%7+3) + string(rune('A'+i/26)))
	}
	line := strings.Repeat("ordinary log line text ", 10)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Mask(line)
	}
}

func TestWriterShortSecretInsideLongerAcrossWrites(t *testing.T) {
	cases := []struct {
		reg    []string
		writes []string
		want   string
	}{
		{[]string{"abcdefgh", "abcdefghSECRETTAIL"}, []string{"x abcdefgh", "SECRETTAIL y"}, "x " + secretmask.Marker + " y"},
		{[]string{"abcdefgh1234", "efgh1234zzzz"}, []string{"x abcdefgh1234", "zzzz y"}, "x " + secretmask.Marker + " y"},
	}
	for _, c := range cases {
		r := secretmask.New()
		for _, v := range c.reg {
			r.Add(v)
		}
		var out bytes.Buffer
		w := secretmask.NewWriterFor(&out, r)
		for _, s := range c.writes {
			_, _ = w.Write([]byte(s))
		}
		_ = w.Close()
		if out.String() != c.want {
			t.Fatalf("got %q want %q", out.String(), c.want)
		}
	}
}
