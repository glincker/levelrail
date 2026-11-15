package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestInternalDNSNamesBodyAndReadBack(t *testing.T) {
	m := nullModel()
	if _, has := m.body(map[string]any{}, false)["dns_names"]; has {
		t.Fatal("unset names must not be sent")
	}
	m.InternalDNSNames = []types.String{strVal("shop"), strVal("www.shop")}
	names, _ := m.body(map[string]any{}, false)["dns_names"].([]string)
	if len(names) != 2 || names[1] != "www.shop" {
		t.Fatalf("body names: %v", names)
	}
	m.InternalDNSNames = []types.String{}
	if got, ok := m.body(map[string]any{}, false)["dns_names"].([]string); !ok || len(got) != 0 {
		t.Fatalf("an empty list must clear: %v", got)
	}

	m.InternalDNSNames = []types.String{strVal("x")}
	m.collectionsFromWire(wireApp{DNSNames: []string{"shop"}})
	if len(m.InternalDNSNames) != 1 || m.InternalDNSNames[0].ValueString() != "shop" {
		t.Fatalf("read back: %v", m.InternalDNSNames)
	}
}
