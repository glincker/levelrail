package main

import (
	"context"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// wireProcfilePort lets the deploy pipeline read the Procfile web port live, since the router that owns the settings is built after the pipeline.
func (s *bootState) wireProcfilePort() {
	if s.builder == nil || s.apiRouter == nil {
		return
	}
	s.builder.SetProcfilePort(func(ctx context.Context) int {
		rs := s.apiRouter.RuntimeSettings()
		if rs == nil {
			return 0
		}
		return rs.Int(ctx, runtimesettings.KeyDeployProcfileDefaultPort)
	})
}
