package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/GLINCKER/levelrail/internal/cloudconn"
)

type ssoFakeSTS struct{}

func (ssoFakeSTS) GetCallerIdentity(context.Context, *sts.GetCallerIdentityInput, ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
	return &sts.GetCallerIdentityOutput{Account: aws.String("111122223333"), Arn: aws.String("arn:aws:sts::111122223333:assumed-role/Admin/x"), UserId: aws.String("u")}, nil
}

func (ssoFakeSTS) AssumeRole(context.Context, *sts.AssumeRoleInput, ...func(*sts.Options)) (*sts.AssumeRoleOutput, error) {
	return &sts.AssumeRoleOutput{}, nil
}

func ssoFakeServer(t *testing.T) *httptest.Server {
	j := func(w http.ResponseWriter, v any) {
		_ = json.NewEncoder(w).Encode(v)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/client/register", func(w http.ResponseWriter, _ *http.Request) {
		j(w, map[string]any{"clientId": "cid", "clientSecret": "csecret", "clientSecretExpiresAt": time.Now().Add(time.Hour).Unix()})
	})
	mux.HandleFunc("/device_authorization", func(w http.ResponseWriter, _ *http.Request) {
		j(w, map[string]any{"deviceCode": "dev", "userCode": "ABCD-EFGH", "verificationUri": "https://v", "verificationUriComplete": "https://v?c=1", "expiresIn": 600, "interval": 1})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		j(w, map[string]any{"accessToken": "access-1", "refreshToken": "refresh-1", "expiresIn": 28800})
	})
	mux.HandleFunc("/assignment/accounts", func(w http.ResponseWriter, _ *http.Request) {
		j(w, map[string]any{"accountList": []map[string]string{{"accountId": "111122223333", "accountName": "prod"}}})
	})
	mux.HandleFunc("/assignment/roles", func(w http.ResponseWriter, _ *http.Request) {
		j(w, map[string]any{"roleList": []map[string]string{{"roleName": "Admin"}}})
	})
	mux.HandleFunc("/federation/credentials", func(w http.ResponseWriter, _ *http.Request) {
		j(w, map[string]any{"roleCredentials": map[string]any{"accessKeyId": "AKIA", "secretAccessKey": "sk", "sessionToken": "st", "expiration": time.Now().UnixMilli() + 3600_000}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestAWSSSOLoginSelectAndRelogin(t *testing.T) {
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_CONFIG_FILE", t.TempDir()+"/none")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", t.TempDir()+"/none")
	srv := ssoFakeServer(t)
	rt, cookie, secrets := connectTestRouter(t, &fakeNodeProvisioner{})
	broker := cloudconn.NewSSOBroker(&cloudconn.SSOClient{HTTP: srv.Client(), OIDCBase: func(string) string { return srv.URL }, PortalBase: func(string) string { return srv.URL }})
	rt.SetCloudProviders(cloudconn.Registry{cloudconn.ProviderAWS: cloudconn.NewAWSProvider(cloudconn.AWSOptions{
		SSO: broker, NewSTS: func(aws.Config) cloudconn.STSAPI { return ssoFakeSTS{} }})})
	do := func(path, body string) (int, string) {
		return teamDo(t, rt, http.MethodPost, "/api/v1/cloud-accounts/aws/sso/"+path, body, withCookie(cookie))
	}
	code, body := do("start", `{"start_url":"http://insecure","region":"eu-west-1"}`)
	if code != http.StatusBadRequest {
		t.Fatalf("insecure start url = %d %s", code, body)
	}
	code, body = do("start", `{"start_url":"https://d-1.awsapps.com/start","region":"eu-west-1"}`)
	var st ssoStartResponse
	if code != http.StatusOK || json.Unmarshal([]byte(body), &st) != nil || st.UserCode != "ABCD-EFGH" || st.SessionID == "" {
		t.Fatalf("start = %d %s", code, body)
	}
	if strings.Contains(body, "dev\"") || strings.Contains(body, "csecret") {
		t.Fatal("start leaked server side material")
	}
	awsSSOSessions.Lock()
	awsSSOSessions.m[st.SessionID].interval = 0
	awsSSOSessions.Unlock()
	code, body = do("poll", `{"session_id":"`+st.SessionID+`"}`)
	var pr ssoPollResponse
	if code != http.StatusOK || json.Unmarshal([]byte(body), &pr) != nil || pr.Status != "done" || len(pr.Accounts) != 1 || pr.Accounts[0].Roles[0] != "Admin" {
		t.Fatalf("poll = %d %s", code, body)
	}
	if strings.Contains(body, "access-1") {
		t.Fatal("poll leaked a token")
	}
	if code, _ = do("select", `{"session_id":"`+st.SessionID+`","account_id":"999999999999","role_name":"Admin"}`); code != http.StatusBadRequest {
		t.Fatalf("unlisted account = %d", code)
	}
	code, body = do("select", `{"session_id":"`+st.SessionID+`","account_id":"111122223333","role_name":"Admin","name":"prod sso","id":"aws-sso-prod"}`)
	var acct cloudAccountWire
	if code != http.StatusCreated || json.Unmarshal([]byte(body), &acct) != nil || acct.Method != cloudconn.AuthAWSSSO || acct.NeedsRelogin {
		t.Fatalf("select = %d %s", code, body)
	}
	sealed := ""
	for _, v := range secrets.values {
		sealed = v
	}
	if !strings.Contains(sealed, "refresh-1") || strings.Contains(body, "refresh-1") {
		t.Fatalf("bundle must be sealed and never returned: %q", sealed)
	}
	// An ended session flags the account in the list and verify, and relogin clears it.
	for k := range secrets.values {
		secrets.values[k] = `{"client_id":"cid","client_secret":"s","needs_relogin":true,"updated_at":1}`
	}
	broker.Forget("aws-sso-prod")
	code, body = teamDo(t, rt, http.MethodGet, "/api/v1/cloud-accounts/aws-sso-prod", "", withCookie(cookie))
	if code != http.StatusOK || !strings.Contains(body, `"needs_relogin":true`) {
		t.Fatalf("get = %d %s", code, body)
	}
	code, body = teamDo(t, rt, http.MethodPost, "/api/v1/cloud-accounts/aws-sso-prod/verify", "", withCookie(cookie))
	if code != http.StatusOK || !strings.Contains(body, cloudconn.ClassNeedsRelogin) {
		t.Fatalf("verify = %d %s", code, body)
	}
	code, body = do("relogin", `{"account_id":"aws-sso-prod"}`)
	if code != http.StatusOK || json.Unmarshal([]byte(body), &st) != nil {
		t.Fatalf("relogin start = %d %s", code, body)
	}
	awsSSOSessions.Lock()
	awsSSOSessions.m[st.SessionID].interval = 0
	awsSSOSessions.Unlock()
	if code, body = do("poll", `{"session_id":"`+st.SessionID+`"}`); code != http.StatusOK || !strings.Contains(body, `"account_id":"aws-sso-prod"`) {
		t.Fatalf("relogin poll = %d %s", code, body)
	}
	code, body = teamDo(t, rt, http.MethodPost, "/api/v1/cloud-accounts/aws-sso-prod/verify", "", withCookie(cookie))
	if code != http.StatusOK || !strings.Contains(body, `"ok":true`) {
		t.Fatalf("verify after relogin = %d %s", code, body)
	}
}
