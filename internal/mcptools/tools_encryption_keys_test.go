package mcptools

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestGetEncryptionKeyStatusTool(t *testing.T) {
	var gotPath string
	age := 40
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.EncryptionKeysStatus{
			MasterKey: apiclient.MasterKeyStatus{AgeDays: &age},
			Apps:      []apiclient.AppKeyStatus{{App: "web", AgeDays: 12, ValueCount: 3}},
			Hints:     []apiclient.EncryptionKeyHint{{Message: "rotate web"}},
		})
	})
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_encryption_key_status"})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	var got encryptionKeyStatusOutput
	decodeStructured(t, result, &got)
	if gotPath != "/api/v1/system/encryption-keys" || got.MasterKeyAgeDays != 40 || len(got.Apps) != 1 || !strings.HasPrefix(got.Apps[0], "web: 12 days") || got.Hints[0] != "rotate web" {
		t.Errorf("path = %q got = %+v", gotPath, got)
	}
}
