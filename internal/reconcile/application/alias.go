package application

import "github.com/GLINCKER/levelrail/internal/store"

// ServiceAlias is the name sibling containers on desired's per-app network
// reach it by.
func ServiceAlias(desired *store.DesiredService) string { return serviceAlias(desired) }
