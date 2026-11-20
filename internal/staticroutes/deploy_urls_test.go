package staticroutes

import "testing"

func TestDeployHost(t *testing.T) {
	if got := DeployHost("web.1-2-3-4.sslip.io", "dep_AbCd-1234xyz"); got != "abcd1234--web.1-2-3-4.sslip.io" {
		t.Errorf("got %q", got)
	}
	if DeployHost("", "dep_AbCd1234") != "" || DeployHost("h", "dep_ab") != "" {
		t.Error("unusable parts must give an empty host")
	}
}

func TestKeepDeploys(t *testing.T) {
	t.Setenv("APP_PAGES_KEEP_DEPLOYS", "1")
	if KeepDeploys() != 3 {
		t.Error("floor is 3")
	}
	t.Setenv("APP_PAGES_KEEP_DEPLOYS", "25")
	if KeepDeploys() != 25 {
		t.Error("env wins")
	}
	t.Setenv("APP_PAGES_KEEP_DEPLOYS", "")
	t.Setenv("APP_STATIC_KEEP_RELEASES", "")
	if KeepDeploys() != 10 {
		t.Error("default is 10")
	}
}
