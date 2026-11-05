package runtimesettings

import (
	"errors"
	"strings"
)

// KeyNodesProtectedTaintKeys lists taint keys whose removal or loosening counts as weakening.
const KeyNodesProtectedTaintKeys = "nodes.protected_taint_keys"

func init() {
	Register(Definition{
		Key: KeyNodesProtectedTaintKeys, Category: CategorySecurity, Kind: KindStringList, Default: "",
		Env: "APP_NODES_PROTECTED_TAINT_KEYS", Apply: ApplyLive, Risk: RiskWeakensSecurity, RemovalIsWeaker: true,
		CheckItem: taintKeyCheck,
		Title:     "Protected node taint keys",
		Description: "Taint keys that keep workloads off sensitive nodes. Removing one of these taints, changing its value or loosening its effect " +
			"counts as weakening, which a stack token can never do; only a human root changes them. Empty protects nothing.",
	})
}

func taintKeyCheck(item string) error {
	if strings.ContainsAny(item, " \t,=") {
		return errors.New("a taint key cannot hold spaces, commas or =")
	}
	return nil
}

// listDropsItem reports whether next is missing an entry of base.
func listDropsItem(base, next []string) bool {
	have := map[string]bool{}
	for _, n := range next {
		have[n] = true
	}
	for _, b := range base {
		if !have[b] {
			return true
		}
	}
	return false
}
