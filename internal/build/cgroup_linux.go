//go:build linux

package build

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

const (
	defaultCgroupRoot = "/sys/fs/cgroup"
	cgroupGroupName   = "levelrail-builds"
	cgroupCPUPeriod   = 100000
)

var cgroupUnsafe = regexp.MustCompile(`[^a-zA-Z0-9_.-]`)

type cgroupV2 struct{ root string }

// NewCgroupProvisioner returns a cgroup v2 provisioner rooted at root, or at
// /sys/fs/cgroup when root is empty.
func NewCgroupProvisioner(root string) CgroupProvisioner {
	if root == "" {
		root = defaultCgroupRoot
	}
	return cgroupV2{root: root}
}

func (c cgroupV2) Provision(id string, memoryMB, cpuMillis int) (string, func(), error) {
	if _, err := os.Stat(filepath.Join(c.root, "cgroup.controllers")); err != nil {
		return "", nil, fmt.Errorf("cgroup v2 is not mounted at %s: %w", c.root, err)
	}
	group := filepath.Join(c.root, cgroupGroupName)
	if err := os.MkdirAll(group, 0o750); err != nil {
		return "", nil, fmt.Errorf("create cgroup %s: %w", group, err)
	}
	// Controllers must be delegated to the parent before a child can set limits.
	if err := os.WriteFile(filepath.Join(group, "cgroup.subtree_control"), []byte("+memory +cpu"), 0o600); err != nil {
		return "", nil, fmt.Errorf("enable cgroup controllers: %w", err)
	}
	name := cgroupUnsafe.ReplaceAllString(id, "_") + "-" + strconv.FormatInt(int64(os.Getpid()), 10)
	dir := filepath.Join(group, name)
	if err := os.Mkdir(dir, 0o750); err != nil && !os.IsExist(err) {
		return "", nil, fmt.Errorf("create cgroup %s: %w", dir, err)
	}
	cleanup := func() { _ = os.Remove(dir) }
	if memoryMB > 0 {
		v := strconv.FormatInt(int64(memoryMB)*1024*1024, 10)
		if err := os.WriteFile(filepath.Join(dir, "memory.max"), []byte(v), 0o600); err != nil {
			cleanup()
			return "", nil, fmt.Errorf("set memory limit: %w", err)
		}
		_ = os.WriteFile(filepath.Join(dir, "memory.swap.max"), []byte("0"), 0o600)
	}
	if cpuMillis > 0 {
		v := fmt.Sprintf("%d %d", cpuMillis*cgroupCPUPeriod/1000, cgroupCPUPeriod)
		if err := os.WriteFile(filepath.Join(dir, "cpu.max"), []byte(v), 0o600); err != nil {
			cleanup()
			return "", nil, fmt.Errorf("set cpu limit: %w", err)
		}
	}
	return filepath.Join(cgroupGroupName, name), cleanup, nil
}
