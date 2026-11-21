package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/awsx"
	"github.com/GLINCKER/levelrail/internal/awsx/awsxtest"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

const awsSvcBase = "/api/v1/cloud-accounts/aws-default/aws/"

func awsServicesRouter(t *testing.T, h func(c awsxtest.Call, n int) awsxtest.Reply) (*Router, *store.DB) {
	t.Helper()
	secrets := newFakeNodeProviderSecrets()
	_ = secrets.SetValue(context.Background(), store.NodeProviderSecretsKey("aws"), store.NodeProviderTokenEnvKey,
		`{"access_key_id":"AKIAEXAMPLE","secret_access_key":"abc","region":"us-east-1"}`)
	db := openTestDB(t)
	rt := NewRouter(slog.New(slog.NewTextHandler(discardWriter{}, nil)), testBrand(), db, WithNodeProviderSecrets(secrets))
	rt.runtimeSettings = runtimesettings.New(db, func(string) (string, bool) { return "", false })
	if err := db.CreateCloudAccount(context.Background(), store.CloudAccount{ID: "aws-default", Provider: "aws", AuthMethod: "static"}); err != nil {
		t.Fatal(err)
	}
	client, _ := awsxtest.New(t, h)
	prev := newAWSXClient
	newAWSXClient = func(awsx.Credentials) (*awsx.Client, error) { return client, nil }
	t.Cleanup(func() { newAWSXClient = prev })
	return rt, db
}

func enableAWSFlag(t *testing.T, rt *Router, key string) {
	t.Helper()
	if _, err := rt.runtimeSettings.Set(context.Background(), key, json.RawMessage("true"), "test"); err != nil {
		t.Fatal(err)
	}
}

func TestCloudAWSServices_FlagOffAnswers409(t *testing.T) {
	rt, db := awsServicesRouter(t, func(awsxtest.Call, int) awsxtest.Reply { return awsxtest.Reply{Body: "{}"} })
	root := loginTestSession(t, rt, db)
	for _, p := range []string{"ecr/repositories", "acm/certificates", "secrets", "audit?resource=x"} {
		code, body := teamDo(t, rt, http.MethodGet, awsSvcBase+p, "", withCookie(root))
		if code != http.StatusConflict || !strings.Contains(body, awsServiceOffMessage) {
			t.Errorf("GET %s with flag off = %d %s", p, code, body)
		}
	}
	if code, _ := teamDo(t, rt, http.MethodPost, awsSvcBase+"ecr/login-token", "", withCookie(root)); code != http.StatusConflict {
		t.Errorf("login-token with flag off = %d", code)
	}
}

func TestCloudAWSServices_SecretsListIsRedactedAndRevealIsAudited(t *testing.T) {
	rt, db := awsServicesRouter(t, func(c awsxtest.Call, _ int) awsxtest.Reply {
		switch {
		case strings.HasSuffix(c.Target, "GetParametersByPath"):
			return awsxtest.Reply{Body: `{"Parameters":[{"Name":"/levelrail/db","Type":"SecureString","Value":"hunter2"}]}`}
		case strings.HasSuffix(c.Target, "ListSecrets"):
			return awsxtest.Reply{Body: `{"SecretList":[{"Name":"levelrail/prod","ARN":"arn:x"}]}`}
		}
		return awsxtest.Reply{Status: 400, Body: `{"__type":"Bad"}`}
	})
	enableAWSFlag(t, rt, runtimesettings.KeyCloudGuardParams)
	root := loginTestSession(t, rt, db)
	code, body := teamDo(t, rt, http.MethodGet, awsSvcBase+"secrets", "", withCookie(root))
	if code != http.StatusOK || strings.Contains(body, "hunter2") || !strings.Contains(body, "[redacted]") || !strings.Contains(body, "/levelrail/db") {
		t.Fatalf("list = %d %s", code, body)
	}
	code, body = teamDo(t, rt, http.MethodPost, awsSvcBase+"secrets/reveal", `{"kind":"ssm","name":"/levelrail/db"}`, withCookie(root))
	if code != http.StatusOK || !strings.Contains(body, "hunter2") {
		t.Fatalf("reveal = %d %s", code, body)
	}
	rows, err := db.ListAuditEntries(context.Background(), 50, nil, store.AuditEntryFilter{Action: "cloud.aws.secrets.reveal"})
	if err != nil || len(rows) != 1 || strings.Contains(rows[0].Detail, "hunter2") {
		t.Fatalf("audit rows = %v err=%v", rows, err)
	}
}

func TestCloudAWSServices_KillSwitchIsRootOnly(t *testing.T) {
	f := newTeamFixture(t)
	rt := f.rt
	rt.runtimeSettings = runtimesettings.New(f.db, func(string) (string, bool) { return "", false })
	if err := f.db.CreateCloudAccount(context.Background(), store.CloudAccount{ID: "aws-default", Provider: "aws", AuthMethod: "static"}); err != nil {
		t.Fatal(err)
	}
	root := loginTestSession(t, rt, f.db)
	dev := f.members[store.TeamRoleDeveloper]
	if err := f.db.UpsertCloudGrant(context.Background(), store.CloudGrant{ID: "cg_ks", AccountID: "aws-default", ResourceKind: store.CloudResourceAccount,
		PrincipalType: store.CloudPrincipalUser, PrincipalID: dev.ID, Actions: []string{store.CloudActionView, store.CloudActionOperate, store.CloudActionManageBudget}}); err != nil {
		t.Fatal(err)
	}
	devCookie := withCookie(f.cookies[store.TeamRoleDeveloper])
	if code, _ := teamDo(t, rt, http.MethodPost, awsSvcBase+"guard/kill-switch", `{"enabled":true}`, devCookie); code != http.StatusForbidden && code != http.StatusNotFound {
		t.Errorf("grant holder toggling kill switch = %d, want 403 or 404", code)
	}
	if rt.cloudGuardConfig().KillSwitch {
		t.Fatal("kill switch changed for a non root caller")
	}
	if code, body := teamDo(t, rt, http.MethodPost, awsSvcBase+"guard/kill-switch", `{"enabled":true}`, withCookie(root)); code != http.StatusOK {
		t.Fatalf("root toggle = %d %s", code, body)
	}
	if !rt.cloudGuardConfig().KillSwitch {
		t.Error("root toggle did not reach the live guard config")
	}
	code, body := teamDo(t, rt, http.MethodGet, awsSvcBase+"guard", "", withCookie(root))
	if code != http.StatusOK || !strings.Contains(body, `"kill_switch":true`) {
		t.Errorf("guard status = %d %s", code, body)
	}
}

func TestCloudAWSServices_NamePrefixScopesSecretsAndParameters(t *testing.T) {
	var sawPath string
	rt, db := awsServicesRouter(t, func(c awsxtest.Call, _ int) awsxtest.Reply {
		if strings.HasSuffix(c.Target, "GetParametersByPath") {
			sawPath = string(c.Body)
			return awsxtest.Reply{Body: `{"Parameters":[]}`}
		}
		if strings.HasSuffix(c.Target, "ListSecrets") {
			return awsxtest.Reply{Body: `{"SecretList":[{"Name":"levelrail/a","ARN":"arn:a"},{"Name":"other/b","ARN":"arn:b"}]}`}
		}
		return awsxtest.Reply{Status: 400, Body: `{"__type":"Bad"}`}
	})
	enableAWSFlag(t, rt, runtimesettings.KeyCloudGuardParams)
	root := loginTestSession(t, rt, db)
	code, body := teamDo(t, rt, http.MethodGet, awsSvcBase+"secrets", "", withCookie(root))
	if code != http.StatusOK || !strings.Contains(sawPath, "/levelrail/") || strings.Contains(body, "other/b") || !strings.Contains(body, "levelrail/a") {
		t.Fatalf("default list = %d %s path=%s", code, body, sawPath)
	}
	for name, c := range map[string]struct{ method, url, body string }{
		"list path":  {http.MethodGet, awsSvcBase + "secrets?path=/other/", ""},
		"put":        {http.MethodPost, awsSvcBase + "secrets", `{"name":"/other/x","value":"v"}`},
		"reveal ssm": {http.MethodPost, awsSvcBase + "secrets/reveal", `{"kind":"ssm","name":"/other/x"}`},
		"reveal sm":  {http.MethodPost, awsSvcBase + "secrets/reveal", `{"kind":"secretsmanager","name":"other/x"}`},
	} {
		code, body := teamDo(t, rt, c.method, c.url, c.body, withCookie(root))
		if code != http.StatusBadRequest || !strings.Contains(body, "levelrail/") {
			t.Fatalf("%s = %d %s", name, code, body)
		}
	}
}
