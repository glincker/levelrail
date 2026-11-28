package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/awsx/awsxtest"
	"github.com/glincker/levelrail/internal/runtimesettings"
)

const lsBase = awsSvcBase + "lightsail/"

func lsOp(c awsxtest.Call) string { return strings.TrimPrefix(c.Target, "Lightsail_20161128.") }

func TestCloudLightsail_FlagOffAnswers409(t *testing.T) {
	rt, db := awsServicesRouter(t, func(awsxtest.Call, int) awsxtest.Reply { return awsxtest.Reply{Body: "{}"} })
	root := loginTestSession(t, rt, db)
	for _, p := range []string{"instances", "bundles", "blueprints", "snapshots", "static-ips"} {
		if code, _ := teamDo(t, rt, http.MethodGet, lsBase+p, "", withCookie(root)); code != http.StatusConflict {
			t.Errorf("GET %s with flag off = %d", p, code)
		}
	}
}

func TestCloudLightsail_DeleteNeedsConfirmAndManaged(t *testing.T) {
	var deletes int
	rt, db := awsServicesRouter(t, func(c awsxtest.Call, _ int) awsxtest.Reply {
		switch lsOp(c) {
		case "GetInstance":
			return awsxtest.Reply{Body: `{"instance":{"name":"web","tags":[]}}`}
		case "DeleteInstance":
			deletes++
		}
		return awsxtest.Reply{Body: "{}"}
	})
	enableAWSFlag(t, rt, runtimesettings.KeyCloudGuardLightsail)
	root := loginTestSession(t, rt, db)
	if code, _ := teamDo(t, rt, http.MethodDelete, lsBase+"instances/web?confirm=nope", "", withCookie(root)); code != http.StatusBadRequest {
		t.Fatalf("wrong confirm = %d", code)
	}
	if code, out := teamDo(t, rt, http.MethodDelete, lsBase+"instances/web?confirm=web", "", withCookie(root)); code != http.StatusConflict || !strings.Contains(out, "not managed") {
		t.Fatalf("unmanaged delete = %d %s", code, out)
	}
	if code, _ := teamDo(t, rt, http.MethodPost, lsBase+"instances/web/stop", "{}", withCookie(root)); code != http.StatusConflict {
		t.Fatalf("unmanaged stop = %d", code)
	}
	if deletes != 0 {
		t.Fatal("a refused delete reached AWS")
	}
}

func TestCloudLightsail_CreateRefusedWithoutBundleAllowlist(t *testing.T) {
	var creates int
	rt, db := awsServicesRouter(t, func(c awsxtest.Call, _ int) awsxtest.Reply {
		if lsOp(c) == "CreateInstances" {
			creates++
		}
		return awsxtest.Reply{Body: `{"bundles":[{"bundleId":"nano_3_0","price":5,"isActive":true}]}`}
	})
	enableAWSFlag(t, rt, runtimesettings.KeyCloudGuardLightsail)
	root := loginTestSession(t, rt, db)
	body, _ := json.Marshal(map[string]string{"name": "n", "availability_zone": "us-east-1a", "blueprint_id": "ubuntu_22_04", "bundle_id": "nano_3_0"})
	if code, out := teamDo(t, rt, http.MethodPost, lsBase+"instances", string(body), withCookie(root)); code != http.StatusConflict || !strings.Contains(out, "allowlist") {
		t.Fatalf("create = %d %s", code, out)
	}
	if creates != 0 {
		t.Fatal("a refused create reached AWS")
	}
}
