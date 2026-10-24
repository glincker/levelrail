package main

import (
	"context"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/bindmount"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// wireBindMountSettings feeds the runtime settings into internal/bindmount, which cannot import the store.
func wireBindMountSettings(rt *api.Router) {
	svc := rt.RuntimeSettings()
	if svc == nil {
		return
	}
	bindmount.SetProvider(func() bindmount.Settings {
		ctx := context.Background()
		return bindmount.Settings{
			BlockCredentialDirs: svc.Bool(ctx, runtimesettings.KeyBindBlockCredentialDirs),
			AllowedRoots:        svc.List(ctx, runtimesettings.KeyBindAllowedRoots),
		}
	})
}
