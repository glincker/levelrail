package api

import (
	"fmt"

	"github.com/glincker/levelrail/internal/diskfree"
)

func statfsBytes(dir string) (free, total int64, err error) {
	if dir == "" {
		return 0, 0, fmt.Errorf("data dir is not configured")
	}
	info, err := diskfree.Stat(dir)
	if err != nil {
		return 0, 0, err
	}
	return info.Free, info.Total, nil
}
