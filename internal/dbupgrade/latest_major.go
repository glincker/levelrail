package dbupgrade

import (
	"context"
	"strconv"
)

// ResolveForMajor returns the newest plain tag whose major is major, for a
// create that must match an existing data volume. ok is false on a lookup miss.
func ResolveForMajor(ctx context.Context, lister TagLister, repo, major string) (string, bool) {
	n, err := strconv.Atoi(major)
	if lister == nil || err != nil {
		return "", false
	}
	tags, err := lister.ListTags(ctx, repo)
	if err != nil {
		return "", false
	}
	best, _ := PickLatest(tags, [][]int{{n}})
	return best, best != ""
}
