//go:build !linux

package appsleep

func establishedOn([]int) int { return 0 }

func portOwner([]int) string { return "" }
