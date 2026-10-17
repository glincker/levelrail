package main

// envMap reports every key as unset so tests never read the real process environment.
func envMap() func(string) (string, bool) {
	return func(string) (string, bool) {
		return "", false
	}
}
