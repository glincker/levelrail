package database

import (
	"bufio"
	"errors"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

const memcachedContainerPort = 11211

// memcachedDefaultMB is memcached's own -m default, used without a limit.
const memcachedDefaultMB = 64

// memcachedMinMB keeps a tiny memory limit from starving the cache entirely.
const memcachedMinMB = 16

// MemcachedCacheMB sizes the item cache at three quarters of the container
// limit: memcached's -m excludes connection and hash table overhead, so a
// cache equal to the limit gets the container OOM killed.
func MemcachedCacheMB(res *store.ServiceResources) int {
	if res == nil || res.MemoryBytes <= 0 {
		return memcachedDefaultMB
	}
	mb := int(res.MemoryBytes * 3 / 4 / (1 << 20))
	if mb < memcachedMinMB {
		return memcachedMinMB
	}
	return mb
}

// memcachedSpec sets the cache size and disables UDP, which memcached
// amplification attacks abuse. No SASL: see docs/managing-databases.md.
func memcachedSpec(res *store.ServiceResources) func(*docker.ContainerSpec) {
	return func(spec *docker.ContainerSpec) {
		spec.Command = []string{"memcached", "-m", strconv.Itoa(MemcachedCacheMB(res)), "-U", "0"}
		spec.Labels[engineConfigLabelKey] = engineConfigRevision(spec)
	}
}

// MemcachedStatsCmd asks the server for its stats over the text protocol.
// The image ships bash but no client, so this uses bash's /dev/tcp.
func MemcachedStatsCmd() []string {
	return []string{"bash", "-c", `exec 3<>/dev/tcp/127.0.0.1/` + strconv.Itoa(memcachedContainerPort) + ` || exit 7
printf 'stats\r\nquit\r\n' >&3
exec cat <&3`}
}

// ParseMemcachedStats reads "STAT name value" lines up to END.
func ParseMemcachedStats(out string) (map[string]string, error) {
	stats := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "END" {
			return stats, nil
		}
		f := strings.Fields(line)
		if len(f) == 3 && f[0] == "STAT" {
			stats[f[1]] = f[2]
		}
	}
	return nil, errors.New("memcached stats answer ended without END")
}
