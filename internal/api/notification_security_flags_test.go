package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/alerting"
)

func TestSecurityNoticeFlagsNeedRoot(t *testing.T) {
	noNotifyDNS(t)
	rt, db, adb := newTestRouterWithNotificationChannels(t)
	seedNotificationChannel(t, adb, "chn_plain", alerting.NotifySlack, "https://hooks.slack.com/services/x")
	writer := seedMatrixToken(t, db, "tok_w", []string{AbilityWrite})
	root := seedMatrixToken(t, db, "tok_r", []string{AbilityRoot})
	do := func(token, method, target, body string) int {
		req := httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		return rec.Code
	}
	login := `{"name":"x","kind":"slack","notify_url":"https://hooks.slack.com/services/x","notify_device_login":true}`
	expired := `{"name":"x","kind":"slack","notify_url":"https://hooks.slack.com/services/x","notify_device_login_expired":true}`
	plain := `{"name":"x","kind":"slack","notify_url":"https://hooks.slack.com/services/x"}`

	for _, body := range []string{login, expired} {
		if got := do(writer, http.MethodPost, "/api/v1/notification-channels", body); got != http.StatusForbidden {
			t.Errorf("write create with a notice flag = %d, want 403", got)
		}
		if got := do(writer, http.MethodPut, "/api/v1/notification-channels/chn_plain", body); got != http.StatusForbidden {
			t.Errorf("write update turning a notice flag on = %d, want 403", got)
		}
	}
	if got := do(writer, http.MethodPost, "/api/v1/notification-channels", plain); got != http.StatusCreated {
		t.Errorf("write create without flags = %d, want 201", got)
	}
	if got := do(root, http.MethodPost, "/api/v1/notification-channels", login); got != http.StatusCreated {
		t.Errorf("root create with a notice flag = %d, want 201", got)
	}
	if got := do(root, http.MethodPut, "/api/v1/notification-channels/chn_plain", expired); got != http.StatusOK {
		t.Errorf("root update turning a notice flag on = %d, want 200", got)
	}
	if got := do(writer, http.MethodPut, "/api/v1/notification-channels/chn_plain", expired); got != http.StatusOK {
		t.Errorf("write keeping an already set flag = %d, want 200", got)
	}
	if got := do(writer, http.MethodPut, "/api/v1/notification-channels/chn_plain", plain); got != http.StatusOK {
		t.Errorf("write turning a flag off = %d, want 200", got)
	}
}
