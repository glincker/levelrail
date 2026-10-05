package authengine

import "runtime/debug"

const libraryModule = "github.com/glincker/theauth-go/v2"

// LibraryVersion reports the linked library version, or "unknown".
func LibraryVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, d := range info.Deps {
		if d.Path == libraryModule {
			if d.Replace != nil && d.Replace.Version != "" {
				return d.Replace.Version
			}
			return d.Version
		}
	}
	return "unknown"
}
