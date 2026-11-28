package application

import "github.com/glincker/levelrail/internal/store"

// ServiceAlias is the name sibling containers on desired's per-app network
// reach it by.
func ServiceAlias(desired *store.DesiredService) string { return serviceAlias(desired) }
