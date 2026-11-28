//go:build live_oci

// Package livetest holds read-only checks against a real tenancy. They never run in CI:
// they need the live_oci build tag and LEVELRAIL_LIVE_OCI_CONFIG (an OCI config file whose
// profile has an API key: user, fingerprint, key_file) plus LEVELRAIL_LIVE_OCI_COMPARTMENT.
package livetest

import (
	"bufio"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/cloudconn"
	"github.com/glincker/levelrail/internal/cloudspend"
	"github.com/glincker/levelrail/internal/provision"
)

func readProfile(t *testing.T, path, profile string) map[string]string {
	t.Helper()
	f, err := os.Open(path) //nolint:gosec // operator supplied path for a manual run
	if err != nil {
		t.Skip("config not readable")
	}
	defer func() { _ = f.Close() }()
	out, cur := map[string]string{}, ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		ln := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(ln, "[") && strings.HasSuffix(ln, "]") {
			cur = ln[1 : len(ln)-1]
			continue
		}
		if k, v, ok := strings.Cut(ln, "="); ok && cur == profile {
			out[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return out
}

func TestLiveOracleReadOnly(t *testing.T) {
	cfgPath, comp := os.Getenv("LEVELRAIL_LIVE_OCI_CONFIG"), os.Getenv("LEVELRAIL_LIVE_OCI_COMPARTMENT")
	if cfgPath == "" || comp == "" {
		t.Skip("set LEVELRAIL_LIVE_OCI_CONFIG and LEVELRAIL_LIVE_OCI_COMPARTMENT")
	}
	profile := os.Getenv("LEVELRAIL_LIVE_OCI_PROFILE")
	if profile == "" {
		profile = "DEFAULT"
	}
	p := readProfile(t, cfgPath, profile)
	if p["user"] == "" || p["key_file"] == "" || p["fingerprint"] == "" {
		t.Skip("profile has no API key (a security token session cannot sign Levelrail requests)")
	}
	pemBytes, err := os.ReadFile(p["key_file"]) //nolint:gosec // operator supplied key path for a manual run
	if err != nil {
		t.Skip("key file not readable")
	}
	cred := cloudconn.OracleCredential{
		TenancyOCID: p["tenancy"], CompartmentOCID: comp, Region: p["region"],
		Secret: cloudconn.NewOracleSecret(p["user"], p["fingerprint"], pemBytes),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	if class, err := cloudconn.NewOracleHTTPChecker().Check(ctx, cred); err != nil || class != cloudconn.ClassOK {
		t.Fatalf("check class %q err %v", class, err)
	}
	caps, err := cloudconn.NewOracleProber().ProbeCapabilities(ctx, cred)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range caps {
		t.Logf("capability %s ok=%v hint=%q", c.Name, c.OK, c.Hint)
	}
	if regions, err := cloudconn.NewOracleProber().Regions(ctx, cred); err != nil {
		t.Logf("regions refused: %v", err)
	} else {
		t.Logf("regions %v", regions)
	}

	drv, err := provision.NewOracle(provision.OracleCredential{
		TenancyOCID: cred.TenancyOCID, UserOCID: p["user"], Fingerprint: p["fingerprint"], PrivateKeyPEM: string(pemBytes),
		CompartmentOCID: comp, Region: cred.Region,
	}, provision.OracleSettings{})
	if err != nil {
		t.Fatal(err)
	}
	list, err := drv.ListInstances(ctx, provision.InstanceScope{Region: cred.Region})
	if err != nil {
		t.Fatalf("list instances: %v", err)
	}
	t.Logf("instances in compartment: %d", len(list))

	rd, err := cloudspend.New(cloudspend.ProviderOracle, cloudspend.Credentials{
		"home_region": cred.Region, "tenancy_ocid": cred.TenancyOCID, "user_ocid": p["user"], "fingerprint": p["fingerprint"], "private_key_pem": string(pemBytes),
	}, func() cloudspend.Config { return cloudspend.Config{MaxPages: 2} })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rd.Spend(ctx, cloudspend.SpendQuery{AccountID: "live"}); err != nil {
		t.Logf("spend read refused: %v", err)
	}
	if br, ok := rd.(interface {
		Budgets(context.Context, cloudspend.SpendQuery) ([]cloudspend.ProviderBudget, error)
	}); ok {
		if b, err := br.Budgets(ctx, cloudspend.SpendQuery{}); err != nil {
			t.Logf("budgets refused: %v", err)
		} else {
			t.Logf("budgets: %d", len(b))
		}
	}
}
