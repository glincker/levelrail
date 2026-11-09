package runtimesettings

// legacyKeys maps a retired setting key to its current name. Add a row here in the same
// change that renames a key, so a stored value follows it instead of silently resetting.
var legacyKeys = map[string]string{}
