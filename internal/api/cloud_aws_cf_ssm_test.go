package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/awsx/awsxtest"
	"github.com/glincker/levelrail/internal/runtimesettings"
)

func TestCloudFrontRoutes_FlagOffAndInvalidate(t *testing.T) {
	rt, db := awsServicesRouter(t, func(_ awsxtest.Call, _ int) awsxtest.Reply {
		return awsxtest.Reply{Status: 201, Body: `<Invalidation><Id>I1</Id><Status>InProgress</Status></Invalidation>`}
	})
	root := loginTestSession(t, rt, db)
	if code, _ := teamDo(t, rt, http.MethodGet, awsSvcBase+"cloudfront/distributions", "", withCookie(root)); code != http.StatusConflict {
		t.Fatalf("flag off = %d", code)
	}
	enableAWSFlag(t, rt, runtimesettings.KeyCloudGuardCloudFront)
	if _, err := rt.runtimeSettings.Set(context.Background(), runtimesettings.KeyCloudFrontMaxPaths, json.RawMessage("1"), "test"); err != nil {
		t.Fatal(err)
	}
	u := awsSvcBase + "cloudfront/distributions/E1/invalidations"
	if code, _ := teamDo(t, rt, http.MethodPost, u, `{"paths":["/a","/b"]}`, withCookie(root)); code != http.StatusBadRequest {
		t.Fatalf("over cap = %d", code)
	}
	code, body := teamDo(t, rt, http.MethodPost, u, `{"paths":["/a"]}`, withCookie(root))
	if code != http.StatusCreated || !strings.Contains(body, "I1") {
		t.Fatalf("invalidate = %d %s", code, body)
	}
}

func TestSsmRoutes_AllowlistGate(t *testing.T) {
	rt, db := awsServicesRouter(t, func(awsxtest.Call, int) awsxtest.Reply {
		return awsxtest.Reply{Body: `{"Command":{"CommandId":"11111111-2222-3333-4444-555555555555"}}`}
	})
	root := loginTestSession(t, rt, db)
	const send = `{"instance":"i-0123456789abcdef0","command":"uptime"}`
	if code, _ := teamDo(t, rt, http.MethodPost, awsSvcBase+"ssm/commands", send, withCookie(root)); code != http.StatusConflict {
		t.Fatalf("flag off = %d", code)
	}
	enableAWSFlag(t, rt, runtimesettings.KeyCloudGuardSSMRun)
	if code, _ := teamDo(t, rt, http.MethodPost, awsSvcBase+"ssm/commands", send, withCookie(root)); code != http.StatusConflict {
		t.Fatalf("empty allowlist = %d", code)
	}
	if _, err := rt.runtimeSettings.Set(context.Background(), runtimesettings.KeyCloudSSMCommands, json.RawMessage(`["uptime"]`), "test"); err != nil {
		t.Fatal(err)
	}
	code, body := teamDo(t, rt, http.MethodPost, awsSvcBase+"ssm/commands", send, withCookie(root))
	if code != http.StatusAccepted || !strings.Contains(body, "11111111") {
		t.Fatalf("allowed = %d %s", code, body)
	}
}
