package iac

import (
	"net/netip"

	"github.com/GLINCKER/levelrail/internal/experimental"
)

const cloudValidateOnlyWarning = "cloud kinds are validated only: this build has no cloud planner installed, so they are not planned or applied"

// isCloudKind reports whether k is one of the cloud provisioning kinds.
func isCloudKind(k Kind) bool {
	switch k {
	case KindCloudAccount, KindCloudNetwork, KindCloudInstance, KindCloudBucket:
		return true
	}
	return false
}

// cloudEnabled reads the experimental cloud flag, which is off by default.
func cloudEnabled() bool { return experimental.Enabled(experimental.Cloud) }

func cloudDisabledIssue(d *Document) Issue {
	return Issue{File: d.File, Line: d.Line, Code: CodeUnavailable,
		Message: "cloud provisioning is not enabled: " + string(d.Kind) + " documents are rejected. " + experimental.DisabledMessage(experimental.Cloud)}
}

// buildCloud checks what the schema cannot (CIDR syntax) and keeps the spec as
// plain fields. Credentials never appear in a document, only a secret name.
func buildCloud(d *Document, s *issueSink) map[string]any {
	var sp map[string]any
	decodeSpec(d, &sp, s)
	if sp == nil {
		sp = map[string]any{}
	}
	if cidr, ok := sp["cidr"].(string); ok {
		if _, err := netip.ParsePrefix(cidr); err != nil {
			s.at("spec.cidr", "cidr %q is not a valid network range", cidr)
		}
	}
	return sp
}

// Minimal valid documents, used as coverage probes and in tests.
const (
	cloudAccountDoc  = "version: 1\nkind: CloudAccount\nmetadata: {name: oci-test}\nspec:\n  provider: oracle\n  region: us-phoenix-1\n  auth: {method: secret_ref, secretRef: oci-key}\n  limits: {monthlyCost: 0, instances: 1}\n"
	cloudNetworkDoc  = "version: 1\nkind: CloudNetwork\nmetadata: {name: lr-net}\nspec: {account: oci-test, cidr: 10.30.0.0/16, ingress: [{port: 443}]}\n"
	cloudInstanceDoc = "version: 1\nkind: CloudInstance\nmetadata: {name: worker-1}\nspec:\n  account: oci-test\n  network: lr-net\n  shape: {cpu: 1, memoryGB: 6, arch: arm64}\n  node: {join: true, labels: {pool: oci}}\n"
	cloudBucketDoc   = "version: 1\nkind: CloudBucket\nmetadata: {name: backups}\nspec: {account: oci-test}\n"
)
