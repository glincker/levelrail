package experimental

// Toggleable reports whether f applies the moment it is switched, with no
// restart. Every other feature is saved at once but wired at startup.
func Toggleable(f Feature) bool {
	return f == ResourceAdvisor || f == AIChat || f == CLIConsole || f == MobileAPI || f == SAMLSCIM
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

// SetRuntime records the Settings choice for a feature. A live feature applies
// now; a startup-wired one only records the choice (see Pending). It never
// turns off a feature the environment enables.
func SetRuntime(f Feature, on bool) {
	mu.Lock()
	defer mu.Unlock()
	desired[f] = on
	if Toggleable(f) {
		runtime[f] = on
	}
}

// ApplyAtBoot records the stored choice for f and makes it the active state,
// before startup wiring reads Enabled.
func ApplyAtBoot(f Feature, on bool) {
	mu.Lock()
	defer mu.Unlock()
	desired[f] = on
	runtime[f] = on
}

// Pending reports whether f is saved in a state a restart has yet to apply.
func Pending(f Feature) bool {
	mu.Lock()
	defer mu.Unlock()
	load()
	if Toggleable(f) {
		return false
	}
	return (desired[f] || enabled[f]) != (enabled[f] || runtime[f])
}

// EnabledByEnv reports whether the environment alone switches f on.
func EnabledByEnv(f Feature) bool {
	mu.Lock()
	defer mu.Unlock()
	load()
	return enabled[f]
}
