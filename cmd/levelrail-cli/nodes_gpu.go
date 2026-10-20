package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// nodeGPUColumn is the GPU cell of "nodes list": free/total GPUs, or "-"
// for a node with none.
func nodeGPUColumn(g *apiclient.NodeGPUResource) string {
	if g == nil || !g.Present {
		return "-"
	}
	cell := fmt.Sprintf("%d/%d free", g.FreeGPUs, g.GPUCount)
	if !g.RuntimeInstalled {
		cell += " (no nvidia runtime)"
	}
	return cell
}

func printNodeGPUHuman(out io.Writer, g *apiclient.NodeGPUResource) {
	if g == nil || !g.Present {
		return
	}
	_, _ = fmt.Fprintf(out, "gpu:\n")
	_, _ = fmt.Fprintf(out, "  gpus:                  %d (%d reserved, %d free)\n", g.GPUCount, g.ReservedGPUs, g.FreeGPUs)
	_, _ = fmt.Fprintf(out, "  vram used/total:       %d/%d MiB\n", g.UsedVRAMMiB, g.TotalVRAMMiB)
	_, _ = fmt.Fprintf(out, "  nvidia runtime:        %t\n", g.RuntimeInstalled)
	if len(g.Reservations) > 0 {
		_, _ = fmt.Fprintf(out, "  reserved by:           %s\n", strings.Join(g.Reservations, ", "))
	}
}
