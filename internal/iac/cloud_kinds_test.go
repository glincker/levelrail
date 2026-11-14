package iac

import (
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/experimental"
)

func buildCloudDocs(t *testing.T, doc string) ([]*Resource, []Issue) {
	t.Helper()
	docs, issues := ParseDocuments([]Source{{Name: "c.yaml", Data: []byte(doc)}})
	if len(issues) > 0 {
		t.Fatal(issues)
	}
	return Build(docs, Options{})
}

func TestCloudKindsRejectedWhenOff(t *testing.T) {
	experimental.Reset()
	for _, doc := range []string{cloudAccountDoc, cloudNetworkDoc, cloudInstanceDoc, cloudBucketDoc} {
		_, issues := buildCloudDocs(t, doc)
		if len(issues) != 1 || issues[0].Code != CodeUnavailable || !strings.Contains(issues[0].Message, "cloud provisioning is not enabled") {
			t.Fatalf("want one unavailable issue, got %+v", issues)
		}
	}
}

func TestCloudKindsValidateWhenOn(t *testing.T) {
	experimental.Set(experimental.Cloud)
	t.Cleanup(experimental.Reset)
	res, issues := buildCloudDocs(t, cloudAccountDoc+"---\n"+cloudNetworkDoc+"---\n"+cloudInstanceDoc+"---\n"+cloudBucketDoc)
	if len(issues) > 0 || len(res) != 4 {
		t.Fatalf("issues %+v resources %d", issues, len(res))
	}
	bad := map[string]string{
		"secret field":  strings.Replace(cloudAccountDoc, "  limits:", "  password: x\n  limits:", 1),
		"inline secret": strings.Replace(cloudAccountDoc, "secretRef: oci-key", "secretRef: oci-key, apiKey: abc", 1),
		"bad cidr":      strings.Replace(cloudNetworkDoc, "10.30.0.0/16", "10.30.0.0", 1),
		"no limits":     strings.Replace(cloudAccountDoc, "  limits: {monthlyCost: 0, instances: 1}\n", "", 1),
	}
	for name, doc := range bad {
		if _, issues := buildCloudDocs(t, doc); len(issues) == 0 {
			t.Errorf("%s was accepted", name)
		}
	}
}
