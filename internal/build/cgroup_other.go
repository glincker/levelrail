//go:build !linux

package build

import "errors"

type cgroupUnsupported struct{}

// NewCgroupProvisioner reports every build as unlimited off Linux, where cgroups do not exist.
func NewCgroupProvisioner(string) CgroupProvisioner { return cgroupUnsupported{} }

func (cgroupUnsupported) Provision(string, int, int) (string, func(), error) {
	return "", nil, errors.New("build resource limits need Linux cgroup v2")
}
