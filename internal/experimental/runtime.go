package experimental

// Toggleable reports whether an operator may switch f on from Settings at
// runtime, on top of the environment.
func Toggleable(f Feature) bool {
	return f == ResourceAdvisor || f == AIChat || f == CLIConsole || f == MobileAPI
}

// ToggleableList returns every feature Settings may switch, sorted.
func ToggleableList() []Feature {
	out := []Feature{}
	for _, f := range All() {
		if Toggleable(f) {
			out = append(out, f)
		}
	}
	return out
}

// SetRuntime records the Settings choice for a toggleable feature. It never
// turns off a feature the environment enables.
func SetRuntime(f Feature, on bool) {
	if !Toggleable(f) {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	runtime[f] = on
}

// EnabledByEnv reports whether the environment alone switches f on.
func EnabledByEnv(f Feature) bool {
	mu.Lock()
	defer mu.Unlock()
	load()
	return enabled[f]
}
