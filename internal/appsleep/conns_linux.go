//go:build linux

package appsleep

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const tcpEstablished = "01"
const tcpListen = "0A"

// establishedOn counts established TCP connections whose local port is in ports.
func establishedOn(ports []int) int {
	if len(ports) == 0 {
		return 0
	}
	want := map[int]bool{}
	for _, p := range ports {
		want[p] = true
	}
	n := 0
	for _, f := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		for _, row := range procTCPRows(f) {
			if row.state == tcpEstablished && want[row.localPort] {
				n++
			}
		}
	}
	return n
}

type tcpRow struct {
	localPort int
	state     string
	inode     string
}

func procTCPRows(path string) []tcpRow {
	f, err := os.Open(path) //nolint:gosec // fixed /proc path
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	var out []tcpRow
	sc := bufio.NewScanner(f)
	sc.Scan()
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 10 {
			continue
		}
		_, port, ok := strings.Cut(fields[1], ":")
		if !ok {
			continue
		}
		p, err := strconv.ParseInt(port, 16, 32)
		if err != nil {
			continue
		}
		out = append(out, tcpRow{localPort: int(p), state: fields[3], inode: fields[9]})
	}
	return out
}

// portOwner names the process listening on one of ports, "" when unknown
// (other users' sockets need root to inspect).
func portOwner(ports []int) string {
	inodes := map[string]int{}
	for _, f := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		for _, row := range procTCPRows(f) {
			for _, p := range ports {
				if row.state == tcpListen && row.localPort == p {
					inodes[row.inode] = p
				}
			}
		}
	}
	if len(inodes) == 0 {
		return ""
	}
	fds, _ := filepath.Glob("/proc/[0-9]*/fd/*")
	for _, fd := range fds {
		link, err := os.Readlink(fd)
		if err != nil || !strings.HasPrefix(link, "socket:[") {
			continue
		}
		ino := strings.TrimSuffix(strings.TrimPrefix(link, "socket:["), "]")
		if port, ok := inodes[ino]; ok {
			pid := strings.Split(fd, "/")[2]
			comm, _ := os.ReadFile("/proc/" + pid + "/comm") //nolint:gosec // pid comes from a /proc glob
			return fmt.Sprintf("%s (pid %s) on port %d", strings.TrimSpace(string(comm)), pid, port)
		}
	}
	return "another process on this server"
}
