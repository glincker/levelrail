package spec

import "fmt"

// ReadOnlyRootfsOn reports an explicit security.read_only_rootfs: true.
func (s *Service) ReadOnlyRootfsOn() bool {
	return s.Security != nil && s.Security.ReadOnlyRootfs != nil && *s.Security.ReadOnlyRootfs
}

// validateSecurity rejects read_only_rootfs combined with settings that
// cannot work on a read-only root.
func (s *Service) validateSecurity(name string) error {
	if !s.ReadOnlyRootfsOn() {
		return nil
	}
	if len(s.Files) > 0 {
		return fmt.Errorf("spec: service %q: security.read_only_rootfs cannot be combined with files, which are copied into the root before start; mount a volume and drop files, or set read_only_rootfs to false", name)
	}
	if s.Build.Type == BuildStatic {
		return fmt.Errorf("spec: service %q: security.read_only_rootfs has no effect on a static build, which runs no app container", name)
	}
	return nil
}
