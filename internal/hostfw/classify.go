package hostfw

import "context"

// InputFiltered classifies whether the backend's input path drops traffic,
// with the same helpers the doctor check uses. known is false for backends
// that are not read as a raw ruleset or when the dump cannot be read.
func InputFiltered(ctx context.Context, run Runner, b Backend) (filtered, known bool) {
	switch b {
	case BackendNftables:
		out, err := run(ctx, "nft", "list", "ruleset")
		if err != nil {
			return false, false
		}
		return NftInputFiltered(string(out)), true
	case BackendIptables:
		out, err := run(ctx, "iptables", "-S")
		if err != nil {
			return false, false
		}
		return IptablesInputFiltered(string(out)), true
	}
	return false, false
}
