package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestApplyKeepsConfiguredLatestAndStoresResolved(t *testing.T) {
	m := databaseModel{Version: types.StringValue("latest")}
	apply(&m, wireDatabase{Engine: "postgres", Version: "18.1"})
	if m.Version.ValueString() != "latest" || m.ResolvedVersion.ValueString() != "18.1" {
		t.Fatalf("version=%q resolved=%q", m.Version.ValueString(), m.ResolvedVersion.ValueString())
	}
}

func TestApplyPinnedAndImportedTakeLiveVersion(t *testing.T) {
	for _, in := range []types.String{types.StringValue("16"), types.StringNull(), types.StringUnknown()} {
		m := databaseModel{Version: in}
		apply(&m, wireDatabase{Engine: "postgres", Version: "16.11"})
		if m.Version.ValueString() != "16.11" || m.ResolvedVersion.ValueString() != "16.11" {
			t.Fatalf("input %v: version=%q", in, m.Version.ValueString())
		}
	}
}
