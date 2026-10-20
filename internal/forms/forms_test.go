package forms

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func testConfig() Config {
	return Config{MaxBodyBytes: 4096, MaxFields: 4, MaxFieldBytes: 64, SpamMaxLinks: 2, MaxStoredPerForm: 100, Retention: 24 * time.Hour}
}

func multipartBody(t *testing.T) (string, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("name", "Ada")
	fw, err := mw.CreateFormFile("resume", "cv.pdf")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write([]byte("%PDF binary"))
	_ = mw.Close()
	return buf.String(), mw.FormDataContentType()
}

func TestParseRequest(t *testing.T) {
	mpBody, mpType := multipartBody(t)
	tests := []struct {
		name    string
		ctype   string
		body    string
		want    map[string]string
		wantErr error
	}{
		{"urlencoded", "application/x-www-form-urlencoded", "name=Ada&tag=a&tag=b", map[string]string{"name": "Ada", "tag": "a, b"}, nil},
		{"json scalars and list", "application/json", `{"name":"Ada","n":3,"tags":["a","b"],"none":null}`, map[string]string{"name": "Ada", "n": "3", "tags": "a, b", "none": ""}, nil},
		{"multipart keeps file name only", mpType, mpBody, map[string]string{"name": "Ada", "resume": "file: cv.pdf"}, nil},
		{"unsupported type", "text/plain", "hi", nil, ErrUnsupportedType},
		{"missing type", "", "a=1", nil, ErrUnsupportedType},
		{"empty body", "application/x-www-form-urlencoded", "", nil, ErrEmpty},
		{"bad json", "application/json", "{", nil, ErrMalformed},
		{"json array root", "application/json", `["a"]`, nil, ErrMalformed},
		{"too many fields", "application/x-www-form-urlencoded", "a=1&b=2&c=3&d=4&e=5", nil, ErrTooManyFields},
		{"field too long", "application/x-www-form-urlencoded", "a=" + strings.Repeat("x", 65), nil, ErrFieldTooLong},
		{"body too large", "application/x-www-form-urlencoded", "a=" + strings.Repeat("x", 5000), nil, ErrTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			if tt.ctype != "" {
				r.Header.Set("Content-Type", tt.ctype)
			}
			got, err := ParseRequest(r, testConfig())
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("field %q = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestClassify(t *testing.T) {
	tests := []struct {
		name       string
		fields     map[string]string
		honeypot   string
		maxLinks   int
		wantSpam   bool
		wantReason string
		wantKeys   int
	}{
		{"clean", map[string]string{"email": "a@b.c"}, "bot-field", 2, false, "", 1},
		{"empty honeypot is fine", map[string]string{"email": "a@b.c", "bot-field": ""}, "bot-field", 2, false, "", 1},
		{"filled honeypot", map[string]string{"email": "a@b.c", "bot-field": "x"}, "bot-field", 2, true, SpamHoneypot, 1},
		{"only honeypot empty", map[string]string{"bot-field": ""}, "bot-field", 2, true, SpamEmpty, 0},
		{"link flood", map[string]string{"m": "http://a https://b http://c"}, "bot-field", 2, true, SpamLinks, 1},
		{"links allowed when heuristic off", map[string]string{"m": "http://a https://b http://c"}, "bot-field", 0, false, "", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clean, spam, reason := Classify(tt.fields, tt.honeypot, tt.maxLinks)
			if spam != tt.wantSpam || reason != tt.wantReason || len(clean) != tt.wantKeys {
				t.Fatalf("got spam=%v reason=%q keys=%d", spam, reason, len(clean))
			}
			if _, ok := clean[tt.honeypot]; ok {
				t.Error("honeypot field must not be stored")
			}
		})
	}
}

func TestValidators(t *testing.T) {
	for name, ok := range map[string]bool{"contact": true, "a_b-1": true, "": false, "Bad": false, "-x": false, "a/b": false, strings.Repeat("a", 64): false} {
		if (ValidateName(name) == nil) != ok {
			t.Errorf("ValidateName(%q) ok=%v, want %v", name, !ok, ok)
		}
	}
	for raw, ok := range map[string]bool{"": true, "https://x.test/thanks": true, "javascript:alert(1)": false, "/relative": false, "https://u:p@x.test": false, "ftp://x.test": false} {
		if (ValidateRedirect(raw) == nil) != ok {
			t.Errorf("ValidateRedirect(%q) ok=%v, want %v", raw, !ok, ok)
		}
	}
	if ValidateHoneypot("9bad") == nil || ValidateHoneypot("") != nil || ValidateHoneypot("website") != nil {
		t.Error("ValidateHoneypot accepted or rejected the wrong names")
	}
}

func TestHashIPNeverRaw(t *testing.T) {
	a, b := HashIP("salt1", "203.0.113.9"), HashIP("salt2", "203.0.113.9")
	if a == b || strings.Contains(a, "203") || len(a) != 16 || HashIP("s", "") != "" {
		t.Fatalf("hash %q %q", a, b)
	}
}

type fakeStore struct {
	added   []store.FormSubmission
	addErr  error
	pruned  int
	pruneFn func(time.Time)
}

func (f *fakeStore) AddFormSubmission(_ context.Context, s store.FormSubmission, _ int) (store.FormSubmission, error) {
	if f.addErr != nil {
		return store.FormSubmission{}, f.addErr
	}
	s.ID = int64(len(f.added) + 1)
	f.added = append(f.added, s)
	return s, nil
}

func (f *fakeStore) PruneFormSubmissions(_ context.Context, c time.Time) (int64, error) {
	f.pruned++
	if f.pruneFn != nil {
		f.pruneFn(c)
	}
	return 0, nil
}

type fakeNotifier struct {
	calls int
	err   error
}

func (n *fakeNotifier) Notify(context.Context, store.Form, store.FormSubmission) error {
	n.calls++
	return n.err
}

func TestSubmitOutcomes(t *testing.T) {
	form := store.Form{ID: "frm_1", AppName: "site", Name: "contact", HoneypotField: "bot-field", NotifyChannelIDs: []string{"ch1"}, IPSalt: "s"}
	tests := []struct {
		name       string
		fields     map[string]string
		addErr     error
		notifyErr  error
		noChannels bool
		wantErr    bool
		wantStored int
		wantNotify int
		wantSpam   bool
	}{
		{"stored and notified", map[string]string{"email": "a@b.c"}, nil, nil, false, false, 1, 1, false},
		{"spam is stored but silent", map[string]string{"email": "a@b.c", "bot-field": "x"}, nil, nil, false, false, 1, 0, true},
		{"notify fails after store", map[string]string{"email": "a@b.c"}, nil, errors.New("slack down"), false, false, 1, 1, false},
		{"store fails means no notify", map[string]string{"email": "a@b.c"}, errors.New("disk full"), nil, false, true, 0, 0, false},
		{"no channels, no notify", map[string]string{"email": "a@b.c"}, nil, nil, true, false, 1, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &fakeStore{addErr: tt.addErr}
			n := &fakeNotifier{err: tt.notifyErr}
			svc := NewService(st, n, testConfig(), slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
			svc.Dispatch = func(fn func()) { fn() }
			f := form
			if tt.noChannels {
				f.NotifyChannelIDs = nil
			}
			sub, err := svc.Submit(context.Background(), f, tt.fields, "203.0.113.9")
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v", err)
			}
			if len(st.added) != tt.wantStored || n.calls != tt.wantNotify {
				t.Fatalf("stored=%d notified=%d, want %d/%d", len(st.added), n.calls, tt.wantStored, tt.wantNotify)
			}
			if tt.wantStored == 1 {
				if sub.Spam != tt.wantSpam || strings.Contains(sub.IPHash, "203") || sub.IPHash == "" {
					t.Errorf("sub = %+v", sub)
				}
			}
		})
	}
}

func TestPruneGate(t *testing.T) {
	st := &fakeStore{}
	svc := NewService(st, nil, testConfig(), nil)
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	svc.Now = func() time.Time { return now }
	form := store.Form{ID: "f", HoneypotField: "bot-field"}
	for i := 0; i < 3; i++ {
		if _, err := svc.Submit(context.Background(), form, map[string]string{"a": "1"}, ""); err != nil {
			t.Fatal(err)
		}
	}
	if st.pruned != 1 {
		t.Fatalf("pruned %d times within the hour, want 1", st.pruned)
	}
	now = now.Add(2 * time.Hour)
	_, _ = svc.Submit(context.Background(), form, map[string]string{"a": "1"}, "")
	if st.pruned != 2 {
		t.Fatalf("pruned %d times after the interval, want 2", st.pruned)
	}
}

func TestWriteCSVNeutralisesFormulas(t *testing.T) {
	var buf bytes.Buffer
	subs := []store.FormSubmission{
		{ID: 2, Fields: map[string]string{"name": "=cmd|' /C calc'!A0", "note": "ok"}, CreatedAt: time.Unix(0, 0)},
		{ID: 1, Fields: map[string]string{"extra": "x"}, Spam: true, CreatedAt: time.Unix(0, 0)},
	}
	if err := WriteCSV(&buf, subs); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "id,created_at,spam,extra,name,note\n") || !strings.Contains(out, "'=cmd") {
		t.Fatalf("csv = %q", out)
	}
}

func TestNoticeTextFlattens(t *testing.T) {
	f := store.Form{Name: "contact", AppName: "site"}
	s := store.FormSubmission{Fields: map[string]string{"msg": "hi @channel\n<b>x</b>  " + strings.Repeat("y", 300)}}
	got := NoticeText(f, s, "https://cp.test/apps/site/forms")
	if strings.Contains(got, "@channel") || strings.Contains(got, "<b>") || strings.Contains(got, "\n\n") || !strings.Contains(got, "...") || !strings.HasSuffix(got, "https://cp.test/apps/site/forms") {
		t.Fatalf("notice = %q", got)
	}
}

func TestLoadConfig(t *testing.T) {
	env := map[string]string{EnvMaxFields: "7", EnvRetentionDays: "bad", EnvRatePerIPPerMinute: "-3"}
	cfg := LoadConfig(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	if cfg.MaxFields != 7 || cfg.Retention != 90*24*time.Hour || cfg.RatePerIPPerMinute != 10 {
		t.Fatalf("cfg = %+v", cfg)
	}
}
