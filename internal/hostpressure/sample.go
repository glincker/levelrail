// Package hostpressure watches CPU and memory pressure on the host and tells the build path to wait or slow down before the kernel starts killing processes.
package hostpressure

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// ErrUnsupported means the host exposes no memory source to read.
var ErrUnsupported = errors.New("hostpressure: no memory source on this host")

// Sample is one reading. PSI is false where /proc/pressure is missing (old kernels, containers, other systems).
type Sample struct {
	PSI       bool    `json:"psi"`
	CPUSome10 float64 `json:"cpu_some_avg10"`
	CPUSome60 float64 `json:"cpu_some_avg60"`
	MemSome10 float64 `json:"mem_some_avg10"`
	MemSome60 float64 `json:"mem_some_avg60"`
	MemFull10 float64 `json:"mem_full_avg10"`
	MemFull60 float64 `json:"mem_full_avg60"`

	MemTotal     int64   `json:"mem_total_bytes"`
	MemAvailable int64   `json:"mem_available_bytes"`
	SwapTotal    int64   `json:"swap_total_bytes"`
	SwapFree     int64   `json:"swap_free_bytes"`
	NumCPU       int     `json:"num_cpu"`
	Load1        float64 `json:"load1"`
}

// ReadFunc returns the current reading; tests inject a fake.
type ReadFunc func() (Sample, error)

// ProcReader reads a /proc shaped tree rooted at Root ("/proc" when empty).
type ProcReader struct {
	Root   string
	NumCPU func() int
}

// Read implements ReadFunc.
func (p ProcReader) Read() (Sample, error) {
	root := p.Root
	if root == "" {
		root = "/proc"
	}
	mem, err := os.ReadFile(filepath.Join(root, "meminfo")) //nolint:gosec // fixed /proc layout, root only overridden by tests
	if err != nil {
		return Sample{}, fmt.Errorf("%w: %w", ErrUnsupported, err)
	}
	var s Sample
	if err := parseMeminfo(mem, &s); err != nil {
		return Sample{}, err
	}
	s.NumCPU = runtime.NumCPU()
	if p.NumCPU != nil {
		s.NumCPU = p.NumCPU()
	}
	if b, err := os.ReadFile(filepath.Join(root, "loadavg")); err == nil { //nolint:gosec // see above
		if f := strings.Fields(string(b)); len(f) > 0 {
			s.Load1, _ = strconv.ParseFloat(f[0], 64)
		}
	}
	cpu, cerr := os.ReadFile(filepath.Join(root, "pressure", "cpu"))   //nolint:gosec // see above
	pm, merr := os.ReadFile(filepath.Join(root, "pressure", "memory")) //nolint:gosec // see above
	if cerr == nil && merr == nil {
		c, e1 := parsePSI(cpu)
		m, e2 := parsePSI(pm)
		if e1 == nil && e2 == nil {
			s.PSI = true
			s.CPUSome10, s.CPUSome60 = c.some10, c.some60
			s.MemSome10, s.MemSome60, s.MemFull10, s.MemFull60 = m.some10, m.some60, m.full10, m.full60
		}
	}
	return s, nil
}

type psiLines struct{ some10, some60, full10, full60 float64 }

// parsePSI reads "some avg10=.. avg60=.." and "full ..." lines.
func parsePSI(data []byte) (psiLines, error) {
	var out psiLines
	seen := false
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 3 || (f[0] != "some" && f[0] != "full") {
			continue
		}
		var a10, a60 float64
		for _, kv := range f[1:] {
			k, v, ok := strings.Cut(kv, "=")
			if !ok {
				continue
			}
			n, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return psiLines{}, fmt.Errorf("hostpressure: bad psi value %q: %w", kv, err)
			}
			switch k {
			case "avg10":
				a10 = n
			case "avg60":
				a60 = n
			}
		}
		if f[0] == "some" {
			out.some10, out.some60 = a10, a60
		} else {
			out.full10, out.full60 = a10, a60
		}
		seen = true
	}
	if !seen {
		return psiLines{}, errors.New("hostpressure: no psi lines")
	}
	return out, nil
}

func parseMeminfo(data []byte, s *Sample) error {
	var haveTotal, haveAvail bool
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		label, rest, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		f := strings.Fields(rest)
		if len(f) == 0 {
			continue
		}
		kb, err := strconv.ParseInt(f[0], 10, 64)
		if err != nil {
			continue
		}
		switch label {
		case "MemTotal":
			s.MemTotal, haveTotal = kb*1024, true
		case "MemAvailable":
			s.MemAvailable, haveAvail = kb*1024, true
		case "SwapTotal":
			s.SwapTotal = kb * 1024
		case "SwapFree":
			s.SwapFree = kb * 1024
		}
	}
	if !haveTotal || !haveAvail {
		return fmt.Errorf("%w: meminfo lacks MemTotal or MemAvailable", ErrUnsupported)
	}
	return nil
}
