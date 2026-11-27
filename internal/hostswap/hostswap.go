// Package hostswap reports a host's RAM and swap and plans an opt-in swapfile.
// Nothing here changes the host unless Apply is called with an explicit request.
package hostswap

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const (
	// DefaultPath is where the helper creates its swapfile.
	DefaultPath = "/swapfile-levelrail"
	// FstabPath is the file the persistent entry is appended to.
	FstabPath = "/etc/fstab"
	mib       = int64(1) << 20
)

// State is what the host reports about memory and swap.
type State struct {
	Supported  bool     `json:"supported"`
	RAMBytes   int64    `json:"ram_bytes"`
	SwapBytes  int64    `json:"swap_bytes"`
	Swapfiles  []string `json:"swapfiles,omitempty"`
	Swappiness int      `json:"swappiness"` // -1 when unknown
}

// Read inspects root+"/proc"; pass "" for the live host.
func Read(root string) State {
	st := State{Swappiness: -1}
	if runtime.GOOS != "linux" && root == "" {
		return st
	}
	f, err := os.Open(filepath.Join(root, "proc", "meminfo")) // #nosec G304 -- fixed proc path
	if err != nil {
		return st
	}
	defer func() { _ = f.Close() }()
	st.Supported = true
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 {
			continue
		}
		kb, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			st.RAMBytes = kb * 1024
		case "SwapTotal:":
			st.SwapBytes = kb * 1024
		}
	}
	st.Swapfiles = readSwapFiles(filepath.Join(root, "proc", "swaps"))
	b, err := os.ReadFile(filepath.Join(root, "proc", "sys", "vm", "swappiness")) // #nosec G304 -- fixed proc path
	if err == nil {
		if n, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil {
			st.Swappiness = n
		}
	}
	return st
}

func readSwapFiles(path string) []string {
	b, err := os.ReadFile(path) // #nosec G304 -- fixed proc path
	if err != nil {
		return nil
	}
	var out []string
	for i, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(line)
		if i == 0 || len(fields) == 0 {
			continue
		}
		out = append(out, fields[0])
	}
	return out
}

// Summary is one plain sentence for doctor rows.
func (s State) Summary() string {
	swap := "no swap"
	if s.SwapBytes > 0 {
		swap = fmt.Sprintf("%d MiB swap", s.SwapBytes/mib)
		if len(s.Swapfiles) > 0 {
			swap += " (" + strings.Join(s.Swapfiles, ", ") + ")"
		}
	}
	wp := "swappiness unknown"
	if s.Swappiness >= 0 {
		wp = "swappiness " + strconv.Itoa(s.Swappiness)
	}
	return fmt.Sprintf("%d MiB RAM, %s, %s", s.RAMBytes/mib, swap, wp)
}

// Plan is the exact set of steps to add a swapfile.
type Plan struct {
	Path     string   `json:"path"`
	SizeMB   int      `json:"size_mb"`
	Commands []string `json:"commands"`
	Notes    []string `json:"notes,omitempty"`
}

// SizeMB resolves the swapfile size: a positive setting wins, otherwise the
// size matches RAM (the usual guidance for small hosts).
func SizeMB(setting int, ramBytes int64) int {
	if setting > 0 {
		return setting
	}
	return int(ramBytes / mib)
}

// BuildPlan refuses when any swap setup exists, the path is taken or free disk
// cannot hold the file twice over (the file plus room to keep running).
func BuildPlan(st State, path string, sizeMB int, freeBytes int64, fstab string) (Plan, error) {
	if path == "" {
		path = DefaultPath
	}
	switch {
	case !st.Supported:
		return Plan{}, errors.New("this host does not expose Linux memory data, so no swapfile plan can be made")
	case st.SwapBytes > 0 || len(st.Swapfiles) > 0:
		return Plan{}, errors.New("swap is already configured; leaving the existing setup alone")
	case sizeMB <= 0:
		return Plan{}, errors.New("swapfile size resolved to zero; set host.swap_size_mb or pass --size-mb")
	}
	if _, err := os.Lstat(path); err == nil {
		return Plan{}, fmt.Errorf("%s already exists; refusing to overwrite it", path)
	}
	if hasSwapEntry(fstab) {
		return Plan{}, fmt.Errorf("%s already lists a swap entry; leaving the existing setup alone", fstab)
	}
	need := int64(sizeMB) * mib * 2
	if freeBytes < need {
		return Plan{}, fmt.Errorf("not enough disk: %d MiB free, %d MiB needed (the file plus the same again as headroom)", freeBytes/mib, need/mib)
	}
	p := Plan{Path: path, SizeMB: sizeMB}
	p.Commands = []string{
		fmt.Sprintf("fallocate -l %dM %s", sizeMB, path),
		"chmod 600 " + path,
		"mkswap " + path,
		"swapon " + path,
		fmt.Sprintf("echo '%s none swap sw 0 0' >> %s", path, fstab),
	}
	if st.Swappiness >= 0 {
		p.Notes = append(p.Notes, fmt.Sprintf("vm.swappiness is %d and is left unchanged; lower it with sysctl only if you want swap used as a last resort", st.Swappiness))
	}
	return p, nil
}

func hasSwapEntry(fstab string) bool {
	b, err := os.ReadFile(fstab) // #nosec G304 -- caller-chosen fstab path
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) >= 3 && !strings.HasPrefix(f[0], "#") && f[2] == "swap" {
			return true
		}
	}
	return false
}

// Apply runs the plan. It needs root on Linux and removes the file again if a
// later step fails, so a failed run leaves the host as it was.
func Apply(ctx context.Context, p Plan, fstab string) error {
	if runtime.GOOS != "linux" {
		return errors.New("applying a swapfile is only supported on Linux")
	}
	if os.Geteuid() != 0 {
		return errors.New("applying a swapfile needs root; rerun with sudo")
	}
	steps := [][]string{
		{"fallocate", "-l", strconv.Itoa(p.SizeMB) + "M", p.Path},
		{"chmod", "600", p.Path},
		{"mkswap", p.Path},
		{"swapon", p.Path},
	}
	for _, s := range steps {
		if out, err := exec.CommandContext(ctx, s[0], s[1:]...).CombinedOutput(); err != nil { // #nosec G204 -- fixed argv, no shell
			_ = exec.CommandContext(ctx, "swapoff", p.Path).Run() // #nosec G204 -- fixed argv
			_ = os.Remove(p.Path)
			return fmt.Errorf("%s failed: %w: %s", s[0], err, strings.TrimSpace(string(out)))
		}
	}
	f, err := os.OpenFile(fstab, os.O_APPEND|os.O_WRONLY, 0) // #nosec G304 -- caller-chosen fstab path
	if err != nil {
		return fmt.Errorf("swap is on for this boot but %s could not be opened to persist it: %w", fstab, err)
	}
	defer func() { _ = f.Close() }()
	if _, err := fmt.Fprintf(f, "%s none swap sw 0 0\n", p.Path); err != nil {
		return fmt.Errorf("swap is on for this boot but the fstab entry was not written: %w", err)
	}
	return nil
}
