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
	// No commit or pid in the name: the parent path is part of the cache key.
	name := cgroupUnsafe.ReplaceAllString(id, "_")
	dir := filepath.Join(group, name)
	if err := os.Mkdir(dir, 0o750); err != nil && !os.IsExist(err) {
		return "", nil, fmt.Errorf("create cgroup %s: %w", dir, err)
	}
	// rmdir fails while processes remain, so a cgroup still in use is kept.
	cleanup := func() { _ = os.Remove(dir) }
	// A reused directory may carry an older build's caps, so every file is rewritten.
	mem, cpu := "max", fmt.Sprintf("max %d", cgroupCPUPeriod)
	if memoryMB > 0 {
		mem = strconv.FormatInt(int64(memoryMB)*1024*1024, 10)
	}
	if cpuMillis > 0 {
		cpu = fmt.Sprintf("%d %d", cpuMillis*cgroupCPUPeriod/1000, cgroupCPUPeriod)
	}
	if err := os.WriteFile(filepath.Join(dir, "memory.max"), []byte(mem), 0o600); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("set memory limit: %w", err)
	}
	// Swap is allowed up to the memory cap so a build cannot swap past it.
	_ = os.WriteFile(filepath.Join(dir, "memory.swap.max"), []byte(mem), 0o600)
	if err := os.WriteFile(filepath.Join(dir, "cpu.max"), []byte(cpu), 0o600); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("set cpu limit: %w", err)
	}
	return filepath.Join(cgroupGroupName, name), cleanup, nil
}
