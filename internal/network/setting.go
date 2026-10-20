package network

import "strings"

// EnvMeshEnabled pins the mesh on or off and wins over the saved setting.
const EnvMeshEnabled = "APP_MESH_ENABLED"

// Sources of the effective mesh setting.
const (
	SettingSourceEnv      = "env"
	SettingSourceSettings = "settings"
	SettingSourceDefault  = "default"
	SettingSourceProfile  = "profile"
)

// ResolveEnabled picks whether the mesh should run: env pins, else the saved
// choice, else off. Any env value but "1" means off, as it always has.
func ResolveEnabled(lookup func(string) (string, bool), saved, savedSet bool) (enabled bool, source string) {
	return ResolveEnabledWithProfile(lookup, saved, savedSet, false)
}

// ResolveEnabledWithProfile is ResolveEnabled with the server profile's mesh
// default in place of a bare off: env, then saved, then the profile.
func ResolveEnabledWithProfile(lookup func(string) (string, bool), saved, savedSet, profileDefault bool) (enabled bool, source string) {
	if v, ok := lookup(EnvMeshEnabled); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v) == "1", SettingSourceEnv
	}
	if savedSet {
		return saved, SettingSourceSettings
	}
	if profileDefault {
		return true, SettingSourceProfile
	}
	return false, SettingSourceDefault
}
