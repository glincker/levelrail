package iac

// LooksSecret is the literal detector resolveLiteral applies to spec env,
// exported so compose attachment scans share it instead of copying patterns.
func LooksSecret(key, value string) bool {
	return looksSecret(key, value)
}
