package email

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResendSender_Send_Success(t *testing.T) {
	var gotAuth string
	var gotReq resendEmailRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "email-123"})
	}))
	t.Cleanup(srv.Close)

	s := resendSender{cfg: ResendConfig{APIKey: "test-key", From: "sender@example.com"}, apiURL: srv.URL}
	if err := s.Send(context.Background(), "to@example.com", "Subject line", "Body text"); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if gotAuth != "Bearer test-key" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer test-key")
	}
	if gotReq.From != "sender@example.com" || len(gotReq.To) != 1 || gotReq.To[0] != "to@example.com" || gotReq.Subject != "Subject line" || gotReq.Text != "Body text" {
		t.Errorf("request body = %+v, want matching from/to/subject/text", gotReq)
	}
}

func TestResendSender_Send_NonSuccessStatusPropagates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"invalid api key"}`))
	}))
	t.Cleanup(srv.Close)

	s := resendSender{cfg: ResendConfig{APIKey: "bad-key", From: "sender@example.com"}, apiURL: srv.URL}
	err := s.Send(context.Background(), "to@example.com", "s", "b")
	if err == nil {
		t.Fatal("Send() error = nil, want an error for a non-2xx response")
	}
}

func TestResendSender_Send_UnreachableServer_ErrorPropagates(t *testing.T) {
	// Same "loopback port nothing listens on" trick
	// TestSMTPSender_UnreachableServer_ErrorPropagates uses: deterministic
	// and network-free, never a real call to api.resend.com.
	s := resendSender{cfg: ResendConfig{APIKey: "key", From: "a@example.com"}, apiURL: "http://127.0.0.1:1"}
	if err := s.Send(context.Background(), "to@example.com", "s", "b"); err == nil {
		t.Fatal("Send() error = nil, want an error when the server is unreachable")
	}
}
