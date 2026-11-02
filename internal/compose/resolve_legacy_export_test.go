package compose

// ResolveMagicVarsLegacy is the SERVICE_-only resolver, kept reachable for the regression test.
var ResolveMagicVarsLegacy = func(f *File, g func(string, string, int) (string, error), p func(string, string, string) error) (map[string][]string, []UnresolvedVar, error) {
	return resolveMagicVars(f, g, p, false)
}
