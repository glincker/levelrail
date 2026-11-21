package cloudconn

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func testTemplateInput() TemplateInput {
	return TemplateInput{
		RoleName: "LevelrailAccess", TrustPrincipalARN: "arn:aws:iam::210987654321:role/levelrail-control", ExternalID: "ext-123456",
		Capabilities: []string{CapInventory, CapCost}, Regions: []string{"eu-west-1", "us-east-1"}, BucketPrefix: "levelrail-",
	}
}

func TestCloudFormationTemplateGolden(t *testing.T) {
	t.Setenv("APP_CLOUD_AWS_TAG_KEY", "")
	got, err := CloudFormationTemplate(testTemplateInput())
	if err != nil {
		t.Fatal(err)
	}
	const golden = "testdata/aws_cfn_inventory_cost.golden.yaml"
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(golden, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("template differs from %s (UPDATE_GOLDEN=1 to refresh)\n%s", golden, got)
	}
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(got), &node); err != nil {
		t.Fatalf("template is not valid YAML: %v", err)
	}
	// The managed policy limit is 6144 non-whitespace characters.
	compact, _ := json.Marshal(BoundaryPolicy(PolicyOptions{Regions: []string{"eu-west-1"}}))
	if n := len(compact); n > 6144 {
		t.Fatalf("boundary policy is %d bytes, over the managed policy limit", n)
	}
}

func TestRenderPolicyFromRegistry(t *testing.T) {
	opt := PolicyOptions{Regions: []string{"us-east-1", "eu-west-1"}, TagKey: "levelrail:managed", TagValue: "true", BucketPrefix: "lr-", RoleName: "R"}
	cases := []struct {
		name    string
		caps    []string
		sid     string
		wantErr bool
		check   func(t *testing.T, st PolicyStatement)
	}{
		{"read tier carries the region limit", []string{CapInventory}, "Inventory1", false, func(t *testing.T, st PolicyStatement) {
			got := st.Condition["StringEquals"]["aws:RequestedRegion"]
			if len(got) != 2 || got[0] != "eu-west-1" {
				t.Fatalf("region condition = %v", got)
			}
		}},
		{"global statement has no region limit", []string{CapCost}, "Cost1", false, func(t *testing.T, st PolicyStatement) {
			if len(st.Condition) != 0 {
				t.Fatalf("cost condition = %v", st.Condition)
			}
		}},
		{"create is guarded by the request tag", []string{CapDeploy}, "Deploy1", false, func(t *testing.T, st PolicyStatement) {
			if v := st.Condition["StringEquals"]["aws:RequestTag/levelrail:managed"]; len(v) != 1 || v[0] != "true" {
				t.Fatalf("request tag = %v", st.Condition)
			}
		}},
		{"delete is guarded by the resource tag", []string{CapDestructive}, "Destructive1", false, func(t *testing.T, st PolicyStatement) {
			if _, ok := st.Condition["StringEquals"]["aws:ResourceTag/levelrail:managed"]; !ok {
				t.Fatalf("resource tag = %v", st.Condition)
			}
		}},
		{"bucket prefix is substituted", []string{CapS3}, "S34", false, func(t *testing.T, st PolicyStatement) {
			if st.Resource[0] != "arn:*:s3:::lr-*/*" {
				t.Fatalf("resource = %v", st.Resource)
			}
		}},
		{"dns zones scoped by ARN without tag", []string{CapDNS}, "Dns2", false, func(t *testing.T, st PolicyStatement) {
			if len(st.Condition) != 0 || st.Resource[0] != "arn:*:route53:::hostedzone/*" {
				t.Fatalf("dns statement = %+v", st)
			}
		}},
		{"s3 tag actions use the request tag", []string{CapS3}, "S32", false, func(t *testing.T, st PolicyStatement) {
			if st.Action[1] != "s3:TagResource" || len(st.Condition["StringEquals"]["aws:RequestTag/levelrail:managed"]) != 1 {
				t.Fatalf("s3 create statement = %+v", st)
			}
		}},
		{"unknown tier", []string{"billing"}, "", true, nil},
	}
	for _, c := range cases {
		doc, err := RenderPolicy(c.caps, opt)
		if c.wantErr {
			if err == nil {
				t.Errorf("%s: want error", c.name)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		var found bool
		for _, st := range doc.Statement {
			if st.Sid == c.sid {
				found = true
				c.check(t, st)
			}
		}
		if !found {
			t.Errorf("%s: sid %s missing", c.name, c.sid)
		}
		if doc.Statement[0].Sid != "Verify1" {
			t.Errorf("%s: verify statement must always come first", c.name)
		}
	}
	js, err := PolicyJSON([]string{CapInventory}, opt)
	if err != nil || !json.Valid([]byte(js)) || strings.Contains(js, "{self_role}") {
		t.Fatalf("policy json = %v, %q", err, js)
	}
}

func TestQuickCreateAndTerraform(t *testing.T) {
	in := testTemplateInput()
	u, err := QuickCreateURL(in, "https://s3.eu-west-1.amazonaws.com/bkt/t.yaml", "levelrail-access", "eu-west-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"https://eu-west-1.console.aws.amazon.com/cloudformation/home?region=eu-west-1#/stacks/create/review?",
		"templateURL=https%3A%2F%2Fs3.eu-west-1.amazonaws.com%2Fbkt%2Ft.yaml", "param_ExternalId=ext-123456", "stackName=levelrail-access",
	} {
		if !strings.Contains(u, want) {
			t.Errorf("url missing %q: %s", want, u)
		}
	}
	if _, err := QuickCreateURL(in, "http://example.test/t.yaml", "s", "eu-west-1"); err == nil {
		t.Error("http template url accepted")
	}
	if _, err := QuickCreateURL(in, "https://example.test/t.yaml", "s", "eu-west-1"); err == nil {
		t.Error("non S3 template url accepted")
	}
	if _, err := QuickCreateURL(in, "https://bkt.s3.eu-west-1.amazonaws.com/t.yaml", "1bad", "eu-west-1"); err == nil {
		t.Error("stack name starting with a digit accepted")
	}
	if _, err := QuickCreateURL(in, "https://bkt.s3.eu-west-1.amazonaws.com/t.yaml", "ok-stack", "eu-west-1"); err != nil {
		t.Errorf("virtual hosted s3 url rejected: %v", err)
	}
	tf, err := TerraformSnippet(in)
	if err != nil || !strings.Contains(tf, `"sts:ExternalId" = var.external_id`) || !strings.Contains(tf, "permissions_boundary") {
		t.Fatalf("terraform = %v\n%s", err, tf)
	}
	bad := in
	bad.ExternalID = "x"
	if _, err := CloudFormationTemplate(bad); err == nil {
		t.Error("short external id accepted")
	}
}

func TestBoundaryDeniesExposureActionsWithinLimit(t *testing.T) {
	doc := BoundaryPolicy(PolicyOptions{Regions: []string{"eu-west-1"}})
	denied := map[string]bool{}
	for _, st := range doc.Statement {
		if st.Effect == "Deny" {
			for _, a := range st.Action {
				denied[a] = true
			}
		}
	}
	for _, a := range []string{
		"rds:ModifyDBSnapshotAttribute", "rds:ModifyDBClusterSnapshotAttribute", "secretsmanager:PutResourcePolicy", "ecr:SetRepositoryPolicy",
		"ssm:CreateDocument", "ssm:UpdateDocument", "ssm:CreateAssociation", "cloudtrail:StopLogging", "cloudtrail:DeleteTrail",
		"cloudtrail:UpdateTrail", "cloudtrail:PutEventSelectors", "logs:DeleteLogGroup", "logs:PutSubscriptionFilter", "cloudfront:UpdateDistribution",
	} {
		if !denied[a] {
			t.Errorf("boundary does not deny %s", a)
		}
	}
	b, _ := json.Marshal(doc)
	n := len(strings.Join(strings.Fields(string(b)), ""))
	if n > 6144 {
		t.Fatalf("boundary has %d non-whitespace characters, limit is 6144", n)
	}
}

func TestRenderPolicyUsesNamePrefix(t *testing.T) {
	doc, err := RenderPolicy([]string{CapDeploy}, PolicyOptions{NamePrefix: "acme/"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(doc)
	if !strings.Contains(string(raw), "secret:acme/*") || !strings.Contains(string(raw), "parameter/acme/*") || strings.Contains(string(raw), "levelrail/*") {
		t.Fatalf("policy not scoped to acme: %s", raw)
	}
}
