package cloudconn

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/glincker/levelrail/internal/secretsource"
)

const testExternalID = "ext-123456"

type fakeSTSServer struct {
	mu       sync.Mutex
	sessions []string
	external []string
	account  string
}

func (f *fakeSTSServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	w.Header().Set("Content-Type", "text/xml")
	switch r.PostForm.Get("Action") {
	case "AssumeRole":
		f.mu.Lock()
		f.sessions = append(f.sessions, r.PostForm.Get("RoleSessionName"))
		f.external = append(f.external, r.PostForm.Get("ExternalId"))
		f.mu.Unlock()
		if r.PostForm.Get("ExternalId") != testExternalID {
			w.WriteHeader(http.StatusForbidden)
			_, _ = fmt.Fprint(w, `<ErrorResponse><Error><Type>Sender</Type><Code>AccessDenied</Code><Message>no</Message></Error><RequestId>r</RequestId></ErrorResponse>`)
			return
		}
		exp := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
		_, _ = fmt.Fprintf(w, `<AssumeRoleResponse><AssumeRoleResult><Credentials><AccessKeyId>ASIATEST</AccessKeyId><SecretAccessKey>s</SecretAccessKey><SessionToken>t</SessionToken><Expiration>%s</Expiration></Credentials><AssumedRoleUser><Arn>arn:aws:sts::%s:assumed-role/LevelrailAccess/x</Arn><AssumedRoleId>AROA:x</AssumedRoleId></AssumedRoleUser></AssumeRoleResult></AssumeRoleResponse>`, exp, f.account)
	case "GetCallerIdentity":
		_, _ = fmt.Fprintf(w, `<GetCallerIdentityResponse><GetCallerIdentityResult><Arn>arn:aws:sts::%s:assumed-role/LevelrailAccess/x</Arn><UserId>AROA:x</UserId><Account>%s</Account></GetCallerIdentityResult></GetCallerIdentityResponse>`, f.account, f.account)
	default:
		w.WriteHeader(http.StatusBadRequest)
	}
}

func awsTestEnv(t *testing.T) {
	t.Helper()
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_CONFIG_FILE", t.TempDir()+"/none")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", t.TempDir()+"/none")
}

func awsTarget(auth, cred string) Target {
	return Target{
		Connection: Connection{
			Name: "prod", Provider: ProviderAWS, AuthMethod: auth, CreatedBy: "ada lovelace",
			Config: Config{Region: "eu-west-1", AccountID: "123456789012", RoleARN: "arn:aws:iam::123456789012:role/LevelrailAccess"},
		},
		Credential: secretsource.NewValue([]byte(cred), ""),
	}
}

func TestAWSCheckRoleAssumptionViaFakeSTS(t *testing.T) {
	awsTestEnv(t)
	fs := &fakeSTSServer{account: "123456789012"}
	srv := httptest.NewServer(fs)
	defer srv.Close()
	p := NewAWSProvider(AWSOptions{BaseEndpoint: srv.URL, PlatformCreds: credentials.NewStaticCredentialsProvider("AKIAPLATFORM", "x", "")})
	ctx := context.Background()

	if class, err := p.Check(ctx, awsTarget(AuthRoleAssumption, testExternalID)); class != ClassOK || err != nil {
		t.Fatalf("check = %q, %v", class, err)
	}
	if fs.sessions[0] != "levelrail-ada-lovelace" || fs.external[0] != testExternalID {
		t.Fatalf("assume saw session %q external %q", fs.sessions[0], fs.external[0])
	}
	id, _, err := p.Identity(ctx, awsTarget(AuthRoleAssumption, testExternalID))
	if err != nil || id.Account != "123456789012" || !strings.Contains(id.ARN, "assumed-role/LevelrailAccess") {
		t.Fatalf("identity = %+v, %v", id, err)
	}

	cases := []struct {
		name  string
		t     Target
		class string
	}{
		{"wrong external id", awsTarget(AuthRoleAssumption, "other-id"), ClassAuthFailed},
		{"no external id", awsTarget(AuthRoleAssumption, ""), ClassCredentialUnresolved},
		{"static keys refused", awsTarget(AuthStatic, `{"access_key_id":"A","secret_access_key":"S"}`), ClassConfigInvalid},
	}
	wrongAcct := awsTarget(AuthRoleAssumption, testExternalID)
	wrongAcct.Connection.Config.AccountID = "999999999999"
	cases = append(cases, struct {
		name  string
		t     Target
		class string
	}{"account mismatch", wrongAcct, ClassConfigInvalid})
	for _, c := range cases {
		class, err := p.Check(ctx, c.t)
		if class != c.class || err == nil {
			t.Errorf("%s: class = %q err = %v, want %q", c.name, class, err, c.class)
		}
		if err != nil && strings.Contains(err.Error(), testExternalID) {
			t.Errorf("%s: error leaks the external id", c.name)
		}
	}
}

func TestAWSStaticKeysNeedOptIn(t *testing.T) {
	awsTestEnv(t)
	p := NewAWSProvider(AWSOptions{})
	cfg := Config{Region: "eu-west-1"}
	if err := p.ValidateConfig(cfg, AuthStatic); err == nil {
		t.Fatal("static accepted without APP_CLOUD_ALLOW_STATIC_KEYS")
	}
	t.Setenv("APP_CLOUD_ALLOW_STATIC_KEYS", "true")
	if err := p.ValidateConfig(cfg, AuthStatic); err != nil {
		t.Fatalf("static with opt in: %v", err)
	}
	if err := p.ValidateConfig(cfg, AuthRoleAssumption); err == nil {
		t.Fatal("role_assumption accepted without role_arn")
	}

	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = fmt.Fprint(w, `<GetCallerIdentityResponse><GetCallerIdentityResult><Arn>arn:aws:iam::123456789012:user/a</Arn><UserId>U</UserId><Account>123456789012</Account></GetCallerIdentityResult></GetCallerIdentityResponse>`)
	}))
	defer srv.Close()
	p = NewAWSProvider(AWSOptions{BaseEndpoint: srv.URL})
	tg := awsTarget(AuthStatic, `{"access_key_id":"AKIASTATICTEST","secret_access_key":"s"}`)
	if class, err := p.Check(context.Background(), tg); class != ClassOK || err != nil {
		t.Fatalf("static check = %q, %v", class, err)
	}
	if !strings.Contains(gotAuth, "AKIASTATICTEST") {
		t.Fatalf("request not signed with the static key: %q", gotAuth)
	}
}

type fakeEC2 struct {
	regions []string
	dry     map[string]string
}

func (f fakeEC2) DescribeRegions(context.Context, *ec2.DescribeRegionsInput, ...func(*ec2.Options)) (*ec2.DescribeRegionsOutput, error) {
	out := &ec2.DescribeRegionsOutput{}
	for _, r := range f.regions {
		out.Regions = append(out.Regions, ec2types.Region{RegionName: aws.String(r)})
	}
	return out, nil
}

type codeErr string

func (c codeErr) Error() string     { return "x" }
func (c codeErr) ErrorCode() string { return string(c) }

func (f fakeEC2) DescribeInstances(context.Context, *ec2.DescribeInstancesInput, ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	return nil, codeErr(f.dry["DescribeInstances"])
}

func (f fakeEC2) CreateSecurityGroup(context.Context, *ec2.CreateSecurityGroupInput, ...func(*ec2.Options)) (*ec2.CreateSecurityGroupOutput, error) {
	return nil, codeErr(f.dry["CreateSecurityGroup"])
}

type fakeSim struct {
	allowed map[string]bool
	err     error
}

func (f fakeSim) Simulate(_ context.Context, req simulateRequest) (map[string]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := map[string]string{}
	for _, a := range req.Actions {
		out[a] = "implicitDeny"
		if f.allowed[a] {
			out[a] = "allowed"
		}
	}
	return out, nil
}

func fakeProvider(e fakeEC2, s PolicySimulator) *AWSProvider {
	fs := &fakeSTSServer{account: "123456789012"}
	srv := httptest.NewServer(fs)
	// The server outlives the test process only until GC; tests are short lived.
	return NewAWSProvider(AWSOptions{
		BaseEndpoint: srv.URL, PlatformCreds: credentials.NewStaticCredentialsProvider("AKIAPLATFORM", "x", ""),
		NewEC2: func(aws.Config) EC2API { return e }, NewSimulator: func(aws.Config) PolicySimulator { return s },
		RegionAllowlist: func(Connection) []string { return []string{"eu-west-1", "us-east-1"} },
	})
}

func TestAWSListRegionsFiltersByAllowlist(t *testing.T) {
	awsTestEnv(t)
	p := fakeProvider(fakeEC2{regions: []string{"us-west-2", "eu-west-1", "us-east-1"}}, nil)
	got, err := p.ListRegions(context.Background(), awsTarget(AuthRoleAssumption, testExternalID))
	if err != nil || !reflect.DeepEqual(got, []string{"eu-west-1", "us-east-1"}) {
		t.Fatalf("regions = %v, %v", got, err)
	}
}

func TestAWSProbeCapabilities(t *testing.T) {
	awsTestEnv(t)
	inv, _ := ResolveCapabilities([]string{CapInventory})
	allowed := map[string]bool{}
	for _, a := range inv[1].ActionList() {
		allowed[a] = true
	}
	tg := awsTarget(AuthRoleAssumption, testExternalID)

	p := fakeProvider(fakeEC2{}, fakeSim{allowed: allowed})
	res, err := p.ProbeCapabilities(context.Background(), tg, CapInventory, CapDeploy)
	if err != nil || len(res) != 2 {
		t.Fatalf("simulate probe = %+v, %v", res, err)
	}
	if !res[0].OK || res[0].Method != ProbeSimulate || res[1].OK || len(res[1].Missing) == 0 || res[1].Hint == "" {
		t.Fatalf("simulate results = %+v", res)
	}

	p = fakeProvider(fakeEC2{dry: map[string]string{"DescribeInstances": "DryRunOperation", "CreateSecurityGroup": "UnauthorizedOperation"}},
		fakeSim{err: codeErr("AccessDenied")})
	res, err = p.ProbeCapabilities(context.Background(), tg, CapInventory, CapDeploy, CapCost)
	if err != nil || len(res) != 3 {
		t.Fatalf("dry run probe = %+v, %v", res, err)
	}
	if !res[0].OK || res[0].Method != ProbeDryRun {
		t.Fatalf("inventory = %+v", res[0])
	}
	if res[1].OK || !reflect.DeepEqual(res[1].Missing, []string{"ec2:CreateSecurityGroup"}) {
		t.Fatalf("deploy = %+v", res[1])
	}
	if res[2].Verified || res[2].Method != ProbeNone {
		t.Fatalf("cost = %+v", res[2])
	}
	if _, err := p.ProbeCapabilities(context.Background(), tg, "nope"); err == nil {
		t.Fatal("unknown capability accepted")
	}
}

func TestPrincipalARN(t *testing.T) {
	for in, want := range map[string]string{
		"arn:aws:sts::123456789012:assumed-role/Role/sess": "arn:aws:iam::123456789012:role/Role",
		"arn:aws:iam::123456789012:user/bob":               "arn:aws:iam::123456789012:user/bob",
		"arn:aws:iam::123456789012:root":                   "",
		"garbage":                                          "",
	} {
		if got := principalARN(in); got != want {
			t.Errorf("principalARN(%q) = %q, want %q", in, got, want)
		}
	}
}
