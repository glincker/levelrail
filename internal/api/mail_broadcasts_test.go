package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/email"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeMailProvider struct {
	mu   sync.Mutex
	sent []email.Message
}

func (f *fakeMailProvider) Send(_ context.Context, to, subject, body string) error {
	return f.SendMessage(context.Background(), email.Message{To: to, Subject: subject, Text: body})
}

func (f *fakeMailProvider) SendMessage(_ context.Context, m email.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, m)
	return nil
}

func (f *fakeMailProvider) messages() []email.Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]email.Message(nil), f.sent...)
}

func mailCall(t *testing.T, rt *Router, token, method, path, body string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestMailBroadcastEndToEndAndUnsubscribe(t *testing.T) {
	db := openTestDB(t)
	provider := &fakeMailProvider{}
	rt := NewRouter(discardLogger(), testBrand(), db, WithEmailSender(provider))
	rt.mail.runner.Delay = 0
	root := seedMatrixToken(t, db, "mail-root", []string{AbilityRoot})
	reader := seedMatrixToken(t, db, "mail-read", []string{AbilityRead})

	if code, _ := mailCall(t, rt, reader, http.MethodPost, "/api/v1/mail/broadcasts", `{}`); code != http.StatusForbidden {
		t.Fatalf("read-only token reached broadcasts: %d", code)
	}
	if code, _ := mailCall(t, rt, root, http.MethodPost, "/api/v1/mail/suppressions", `{"email":"held@example.com"}`); code != http.StatusCreated {
		t.Fatalf("suppress: %d", code)
	}

	body := `{"subject":"Hello {{first_name}}","body":"News for {{display_name}}","recipients":["ann@example.com","held@example.com"],"dry_run":true}`
	code, plan := mailCall(t, rt, root, http.MethodPost, "/api/v1/mail/broadcasts", body)
	if code != http.StatusOK || plan["deliverable"] != float64(1) || plan["suppressed"] != float64(1) {
		t.Fatalf("dry run = %d %v", code, plan)
	}
	if len(provider.messages()) != 0 {
		t.Fatal("dry run sent mail")
	}

	sendBody := strings.Replace(body, `"dry_run":true`, `"expected_recipients":1`, 1)
	code, created := mailCall(t, rt, root, http.MethodPost, "/api/v1/mail/broadcasts", sendBody)
	if code != http.StatusCreated {
		t.Fatalf("send = %d %v", code, created)
	}
	id, _ := created["id"].(string)
	deadline := time.Now().Add(5 * time.Second)
	for {
		b, err := db.GetMailBroadcast(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if b.Status == store.MailBroadcastCompleted {
			if b.Sent != 1 || b.Suppressed != 1 || b.Failed != 0 {
				t.Fatalf("counts sent=%d suppressed=%d failed=%d", b.Sent, b.Suppressed, b.Failed)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("broadcast stuck in %s", b.Status)
		}
		time.Sleep(10 * time.Millisecond)
	}

	msgs := provider.messages()
	if len(msgs) != 1 || msgs[0].To != "ann@example.com" || msgs[0].HTML == "" {
		t.Fatalf("messages = %+v", msgs)
	}
	header := msgs[0].Headers["List-Unsubscribe"]
	if msgs[0].Headers["List-Unsubscribe-Post"] == "" || !strings.HasPrefix(header, "<") {
		t.Fatalf("missing unsubscribe headers: %v", msgs[0].Headers)
	}
	link := strings.Trim(header, "<>")

	getRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(getRec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, link, nil))
	if getRec.Code != http.StatusOK || !strings.Contains(getRec.Body.String(), "Unsubscribe") {
		t.Fatalf("unsubscribe page = %d", getRec.Code)
	}
	postRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(postRec, httptest.NewRequestWithContext(context.Background(), http.MethodPost, link, nil))
	if postRec.Code != http.StatusOK {
		t.Fatalf("one-click unsubscribe = %d", postRec.Code)
	}
	sup, err := db.ListMailSuppressions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range sup {
		found = found || s.Email == "ann@example.com"
	}
	if !found {
		t.Fatalf("ann not suppressed after unsubscribe: %+v", sup)
	}

	badRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(badRec, httptest.NewRequestWithContext(context.Background(), http.MethodPost, unsubscribePath+"?t=forged.AAAA", nil))
	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("forged token = %d", badRec.Code)
	}
}

func TestMailBroadcastRefusedWithoutProvider(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db)
	root := seedMatrixToken(t, db, "mail-root2", []string{AbilityRoot})
	code, out := mailCall(t, rt, root, http.MethodPost, "/api/v1/mail/broadcasts", `{"subject":"s","body":"b","recipients":["a@example.com"]}`)
	if code != http.StatusConflict || out["code"] != "email_not_configured" {
		t.Fatalf("got %d %v", code, out)
	}
}

func TestMailTemplateSaveRejectsUnknownTokenAndKeepsVersions(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db)
	root := seedMatrixToken(t, db, "mail-root3", []string{AbilityRoot})

	code, _ := mailCall(t, rt, root, http.MethodPut, "/api/v1/mail/templates/password_reset", `{"subject":"x","body":"{{nope}}"}`)
	if code != http.StatusBadRequest {
		t.Fatalf("unknown token accepted: %d", code)
	}
	for i := 0; i < 12; i++ {
		body := `{"subject":"Reset v` + string(rune('a'+i)) + `","body":"Go: {{reset_link}}"}`
		if code, _ := mailCall(t, rt, root, http.MethodPut, "/api/v1/mail/templates/password_reset", body); code != http.StatusOK {
			t.Fatalf("save %d = %d", i, code)
		}
	}
	vs, err := db.ListMailTemplateVersions(context.Background(), "password_reset")
	if err != nil || len(vs) != store.MailTemplateVersionsKept {
		t.Fatalf("versions = %d, %v", len(vs), err)
	}
	if code, _ := mailCall(t, rt, root, http.MethodDelete, "/api/v1/mail/templates/password_reset", ""); code != http.StatusOK {
		t.Fatalf("reset = %d", code)
	}
}
