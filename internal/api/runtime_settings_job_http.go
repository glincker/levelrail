package api

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/scheduledtask"
)

func init() {
	runtimesettings.Register(runtimesettings.Definition{
		Key: scheduledtask.KeyHTTPTimeoutSeconds, Category: runtimesettings.CategoryDeploys, Kind: runtimesettings.KindInt,
		Default: "30", Env: scheduledtask.EnvHTTPTimeoutSeconds, Apply: runtimesettings.ApplyLive, EnvLoose: true,
		CheckValue:  checkJobHTTPTimeout,
		Title:       "HTTP job timeout",
		Description: "Seconds an HTTP job waits for its app to answer when the job sets no timeout of its own. Read on every run; the environment variable only seeds the default.",
	})
}

// wireJobHTTPSettings feeds the saved timeout to the scheduler so it applies live.
func wireJobHTTPSettings(svc *runtimesettings.Service) {
	scheduledtask.SetHTTPTimeoutResolver(func() time.Duration {
		return time.Duration(svc.Int(context.Background(), scheduledtask.KeyHTTPTimeoutSeconds)) * time.Second
	})
}

func checkJobHTTPTimeout(canonical string) error {
	if n, err := strconv.Atoi(canonical); err != nil || n < 1 || n > 3600 {
		return fmt.Errorf("value must be between 1 and 3600")
	}
	return nil
}
