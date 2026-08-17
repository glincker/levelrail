package main

// envMap is a lookupEnv func (the same shape os.LookupEnv has) that
// reports every key as unset, so tests don't touch real process
// environment variables. Env-var precedence itself is now tested in
// internal/apiclient, which every call site here only exercises
// indirectly through resolveToken/resolveAPIURL's flag/credentials-file
// paths, never a real env var.
func envMap() func(string) (string, bool) {
	return func(string) (string, bool) {
		return "", false
	}
}
