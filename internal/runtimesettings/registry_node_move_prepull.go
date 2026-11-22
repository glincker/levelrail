package runtimesettings

import "time"

// Keys for pre-pulling a moved resource's image on the target node.
const (
	KeyNodeMovePrepull        = "node_move.prepull"
	KeyNodeMovePrepullTimeout = "node_move.prepull_timeout"
)

func init() {
	Register(Definition{
		Key: KeyNodeMovePrepull, Category: CategoryDeploys, Kind: KindBool, Default: "true",
		Apply:       ApplyLive,
		Title:       "Pre-pull the image before a move stops the source",
		Description: "A move or drain pulls the image on the target node before it stops the source, so a large image does not add to the downtime. A failed pre-pull never fails the move. Images built on the control plane and images that need a stored registry login are not pre-pulled. The next move uses a changed value.",
	})
	Register(Definition{
		Key: KeyNodeMovePrepullTimeout, Category: CategoryDeploys, Kind: KindDuration, Default: "10m",
		Apply: ApplyLive, CheckValue: durationRangeCheck(10*time.Second, time.Hour),
		Title:       "Image pre-pull time limit for a move",
		Description: "How long a move waits for the target node to pull the image before it goes on without it, from 10s to 1h. The target then pulls the rest when the resource starts.",
	})
}
