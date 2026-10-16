package importplan

// StackGuess is a stack detected from manifest files alone.
type StackGuess struct {
	Name   string
	Method BuildMethod
	Port   int
	Health string
	Start  string
	Why    string
}

// GuessStack inspects manifest files through read, in a fixed order, to pick a stack.
func GuessStack(read func(string) (string, bool, error)) (StackGuess, bool) {
	g, ok := guessFramework(read)
	return StackGuess{Name: g.Name, Method: g.Method, Port: g.Port, Health: g.Health, Start: g.Start, Why: g.Why}, ok
}

// ParseDockerfile extracts the EXPOSE ports and ENV entries of a Dockerfile.
func ParseDockerfile(text string) (ports []int, env []EnvVar) {
	return parseDockerfile(text)
}

// KeyLooksSecret reports whether an env variable name suggests a secret.
func KeyLooksSecret(key string) bool {
	return secretKeyRe.MatchString(key)
}
