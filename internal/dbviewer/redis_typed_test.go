package dbviewer

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

// redisStubExec answers by the first command word of the stdin script.
type redisStubExec struct{ by map[string]string }

func (s redisStubExec) ExecWithInput(_ context.Context, _ string, _ []string, in io.Reader) (io.ReadCloser, error) {
	b, _ := io.ReadAll(in)
	first := strings.SplitN(string(b), "\n", 2)[0]
	for prefix, out := range s.by {
		if strings.HasPrefix(first, prefix) {
			return io.NopCloser(strings.NewReader(out)), nil
		}
	}
	return io.NopCloser(strings.NewReader("")), nil
}

func redisTestTarget(by map[string]string) Target {
	return Target{Exec: redisStubExec{by: by}, Limits: Limits{Timeout: time.Second, MaxRows: 50, MaxBytes: 1 << 16, MaxCellBytes: 1024}}
}

func TestRedisGetKeyTypedStream(t *testing.T) {
	tg := redisTestTarget(map[string]string{
		"TYPE": "stream\n(integer) -1\nstream\n(integer) 512\n",
		"XLEN": "(integer) 2\n",
		"XRANGE": `1) 1) "1-0"
   2) 1) "name"
      2) "ada"
      3) "age"
      4) "123-4"
2) 1) "2-0"
   2) 1) "name"
      2) "bob"
`,
	})
	v, err := tg.GetKey(context.Background(), "s")
	if err != nil || v.Type != "stream" || v.Size != 2 || v.MemoryBytes != 512 || v.Encoding != "stream" {
		t.Fatalf("v=%+v err=%v", v, err)
	}
	if len(v.Entries) != 2 || v.Entries[0].ID != "1-0" || len(v.Entries[0].Pairs) != 2 || v.Entries[0].Pairs[1].Value != "123-4" || v.Entries[1].Pairs[0].Value != "bob" {
		t.Fatalf("entries=%+v", v.Entries)
	}
}

func TestRedisGetKeyHashZsetAndMissing(t *testing.T) {
	tg := redisTestTarget(map[string]string{
		"TYPE":  "hash\n(integer) 100\n\"listpack\"\n(nil)\n",
		"HLEN":  "(integer) 1\n",
		"HSCAN": "1) \"0\"\n2) 1) \"f\"\n   2) \"x\\ny\"\n",
	})
	v, err := tg.GetKey(context.Background(), "h")
	if err != nil || v.MemoryBytes != -1 || len(v.Entries) != 1 || v.Entries[0].Field != "f" || v.Entries[0].Value != "x\ny" || v.TTL != 100 {
		t.Fatalf("v=%+v err=%v", v, err)
	}
	z := typedEntries("zset", []string{"a", "1.5", "b", "2"})
	if len(z) != 2 || z[0].Value != "a" || z[0].Score != "1.5" {
		t.Fatalf("zset=%+v", z)
	}
	if _, err := redisTestTarget(map[string]string{"TYPE": "none\n(integer) -2\n(nil)\n(nil)\n"}).GetKey(context.Background(), "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err=%v", err)
	}
}

func TestRedisGetKeyJSONUnsupportedAndHLL(t *testing.T) {
	tg := redisTestTarget(map[string]string{
		"TYPE":       "ReJSON-RL\n(integer) -1\n\"raw\"\n(integer) 40\n",
		"JSON.DEBUG": "(error) ERR unknown command 'JSON.GET', with args beginning with: 'k'\n",
	})
	v, err := tg.GetKey(context.Background(), "k")
	if err != nil || !v.Unsupported {
		t.Fatalf("v=%+v err=%v", v, err)
	}
	tg = redisTestTarget(map[string]string{
		"TYPE": "string\n(integer) -1\n\"raw\"\n(integer) 90\n", "STRLEN": "(integer) 90\n", "GET": "\"HYLL\\x00junk\"\n",
	})
	v, err = tg.GetKey(context.Background(), "h")
	if err != nil || v.Subtype != "hyperloglog" || strings.Contains(v.Lines[0], "junk") {
		t.Fatalf("v=%+v err=%v", v, err)
	}
}

// recExec records every script so tests can assert which commands were sent.
type recExec struct {
	redisStubExec
	sent *[]string
}

func (r recExec) ExecWithInput(ctx context.Context, c string, a []string, in io.Reader) (io.ReadCloser, error) {
	b, _ := io.ReadAll(in)
	*r.sent = append(*r.sent, string(b))
	return r.redisStubExec.ExecWithInput(ctx, c, a, strings.NewReader(string(b)))
}

func TestRedisGetKeyBoundsLargeString(t *testing.T) {
	var sent []string
	tg := redisTestTarget(nil)
	tg.Exec = recExec{redisStubExec{by: map[string]string{
		"TYPE": "string\n(integer) -1\n\"raw\"\n(integer) 536870912\n", "STRLEN": "(integer) 536870912\n", "GETRANGE": "\"abc\"\n",
	}}, &sent}
	v, err := tg.GetKey(context.Background(), "big")
	if err != nil || !v.Truncated {
		t.Fatalf("v=%+v err=%v", v, err)
	}
	all := strings.Join(sent, "|")
	if !strings.Contains(all, "GETRANGE") || strings.Contains(all, "GET \"big\"") || strings.Contains(all, "BITCOUNT") {
		t.Fatalf("sent=%q", all)
	}
}

func TestRedisGetKeyJSONTooBigNotFetched(t *testing.T) {
	var sent []string
	tg := redisTestTarget(nil)
	tg.Exec = recExec{redisStubExec{by: map[string]string{
		"TYPE": "ReJSON-RL\n(integer) -1\n\"raw\"\n(integer) 9\n", "JSON.DEBUG": "(integer) 99999999\n",
	}}, &sent}
	v, err := tg.GetKey(context.Background(), "j")
	if err != nil || !v.Truncated || strings.Contains(strings.Join(sent, "|"), "JSON.GET") {
		t.Fatalf("v=%+v err=%v sent=%v", v, err, sent)
	}
}

func TestRedisReadCountShrinksForBigElements(t *testing.T) {
	tg := redisTestTarget(nil)
	if n := tg.readCount(RedisValue{Size: 10, MemoryBytes: 10 * (1 << 20)}); n >= tg.Limits.MaxRows || n < 1 {
		t.Fatalf("n=%d", n)
	}
}

func TestRedisScanKeysNewlineKey(t *testing.T) {
	tg := redisTestTarget(map[string]string{
		"SCAN": "1) \"0\"\n2) 1) \"a\\nb\"\n   2) \"c\"\n",
		"TYPE": "string\n(integer) -1\nhash\n(integer) 5\n",
	})
	sc, err := tg.ScanKeys(context.Background(), "0", "*", 10)
	if err != nil || len(sc.Keys) != 2 || sc.Keys[0].Key != "a\nb" || sc.Keys[1].Type != "hash" || sc.Keys[1].TTL != 5 {
		t.Fatalf("sc=%+v err=%v", sc, err)
	}
}
