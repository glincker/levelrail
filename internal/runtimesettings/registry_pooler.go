package runtimesettings

import (
	"fmt"
	"strings"
)

// KeyDatabasesPoolerImage is the image the managed Postgres pooler runs.
const KeyDatabasesPoolerImage = "databases.pooler_image"

// DefaultPoolerImage is an exact version tag of a pgbouncer image that speaks
// the env contract the pooler reconciler writes.
const DefaultPoolerImage = "edoburu/pgbouncer:v1.26.1-p0"

func poolerImageCheck(canonical string) error {
	v := strings.TrimSpace(canonical)
	if v == "" || strings.ContainsAny(v, " \t\n") {
		return fmt.Errorf("must be an image reference without spaces")
	}
	if strings.HasSuffix(v, ":latest") || (!strings.Contains(v, "@sha256:") && !strings.Contains(v[strings.LastIndex(v, "/")+1:], ":")) {
		return fmt.Errorf("must pin a version tag or digest, not latest")
	}
	return nil
}

func init() {
	Register(Definition{
		Key: KeyDatabasesPoolerImage, Category: CategoryDeploys, Kind: KindString, Default: DefaultPoolerImage,
		Env: "APP_PG_POOLER_IMAGE", Apply: ApplyLive, CheckValue: poolerImageCheck,
		Title:       "Postgres pooler image",
		Description: "The pinned image tag or digest the connection pooler runs. It must speak the pgbouncer env contract. Applies to poolers created or reconciled after the change.",
	})
}
