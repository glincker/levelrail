package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/secretmask"
)

func TestAISSESinkMasksProposedArguments(t *testing.T) {
	secret := "sk-proposal-secret-value-123456" //nolint:gosec // synthetic test value
	secretmask.Add(secret)
	rec := httptest.NewRecorder()
	sink := &aiSSESink{w: rec, flusher: rec}
	sink.ToolCallProposed("conf_1", "t1", "set_app_env", json.RawMessage(`{"value":"`+secret+`","key":"DB"}`))

	body := rec.Body.String()
	if strings.Contains(body, secret) {
		t.Fatalf("proposal event leaked the secret: %s", body)
	}
	if !strings.Contains(body, `"key":"DB"`) {
		t.Errorf("proposal event lost non-secret arguments: %s", body)
	}
}
