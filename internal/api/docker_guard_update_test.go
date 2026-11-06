package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/dockerguard"
)

func TestUpdateDockerGuardInvalidConfigIs409WithARestartHint(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rt.SetDockerGuard(&fakeDockerGuard{setErr: fmt.Errorf("%w: bad json", dockerguard.ErrConfigInvalid)})
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/system/docker-guard", `{"mode":"audit"}`))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "restart") {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}
