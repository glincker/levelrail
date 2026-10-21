package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestProxyInstallWithoutDockerIsAConflict(t *testing.T) {
	env := newProxyTestEnv(t)
	for _, body := range []string{`{}`, `{"provider":"traefik","confirm":true}`} {
		code, raw := env.do(t, http.MethodPost, "/api/v1/system/proxy-integration/install", body)
		var c proxyConflict
		_ = json.Unmarshal(raw, &c)
		if code != http.StatusConflict || c.Code != "docker_unavailable" {
			t.Fatalf("body %s: got %d %s", body, code, raw)
		}
	}
}
