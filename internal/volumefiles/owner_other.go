//go:build windows

package volumefiles

func (s *Store) matchOwner(_, _ string) error { return nil }
