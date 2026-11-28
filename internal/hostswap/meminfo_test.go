package hostswap

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const ubuntuMeminfo = `MemTotal:        6043988 kB
MemFree:          412304 kB
MemAvailable:    4981120 kB
Buffers:          201336 kB
Cached:          4012448 kB
SwapCached:            0 kB
SwapTotal:             0 kB
SwapFree:              0 kB
HugePages_Total:       0
HugePages_Free:        0
HugePages_Rsvd:        0
HugePages_Surp:        0
Hugepagesize:      65536 kB
Hugetlb:               0 kB
`

func TestReadRealMeminfoSamples(t *testing.T) {
	tests := []struct {
		name      string
		meminfo   string
		swaps     string
		wantRAM   int64
		wantSwap  int64
		wantFiles int
	}{
		{name: "ubuntu aarch64 no swap", meminfo: ubuntuMeminfo, swaps: "Filename Type Size Used Priority\n", wantRAM: 6043988 * 1024},
		{name: "swap active", meminfo: strings.Replace(ubuntuMeminfo, "SwapTotal:             0 kB", "SwapTotal:       4194300 kB", 1), swaps: "Filename Type Size Used Priority\n/swapfile file 4194300 0 -2\n", wantRAM: 6043988 * 1024, wantSwap: 4194300 * 1024, wantFiles: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := fakeProc(t, 0, tt.swaps)
			if err := os.WriteFile(filepath.Join(root, "proc", "meminfo"), []byte(tt.meminfo), 0o600); err != nil {
				t.Fatal(err)
			}
			st := Read(root)
			if !st.Supported || st.RAMBytes != tt.wantRAM || st.SwapBytes != tt.wantSwap || len(st.Swapfiles) != tt.wantFiles {
				t.Errorf("state = %+v", st)
			}
		})
	}
}

func TestReadLiveHostUsesAbsoluteProc(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("live /proc only on linux")
	}
	if st := Read(""); !st.Supported || st.RAMBytes == 0 {
		t.Errorf("live read = %+v", st)
	}
}
