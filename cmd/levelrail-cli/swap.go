package main

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/hostswap"
	"github.com/GLINCKER/levelrail/kit/diskspace"
)

const (
	envHostSwapSizeMB   = "APP_HOST_SWAP_SIZE_MB"
	envHostNoSwapRAMMB  = "APP_HOST_NO_SWAP_RAM_THRESHOLD_MB"
	keyHostSwapSizeMB   = "host.swap_size_mb"
	defaultNoSwapRAMMB  = 16384
	swapDiskProbeFolder = "/"
)

type swapReport struct {
	State   hostswap.State `json:"state"`
	Plan    *hostswap.Plan `json:"plan,omitempty"`
	Applied bool           `json:"applied"`
	Reason  string         `json:"reason,omitempty"`
}

// runSwap prints the swapfile plan for this host and changes nothing unless --apply is given.
func runSwap(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "swap", "print the plan as JSON to stdout and nothing else", stderr)
	apply := fs.Bool("apply", false, "create the swapfile now (needs root on this host); without it the commands are only printed")
	sizeMB := fs.Int("size-mb", 0, "swapfile size in MiB (default: the host.swap_size_mb setting, then the host RAM)")
	path := fs.String("path", hostswap.DefaultPath, "where to create the swapfile")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	st := hostswap.Read("")
	size := *sizeMB
	if size == 0 {
		size = configuredSwapSizeMB(tokenFlag, apiURLFlag, profileFlag, prog, lookupEnv)
	}
	free, err := diskspace.Free(swapDiskProbeFolder)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, newValidationError("could not read free disk space: %v", err))
	}
	rep := swapReport{State: st}
	plan, perr := hostswap.BuildPlan(st, *path, hostswap.SizeMB(size, st.RAMBytes), free, hostswap.FstabPath)
	if perr != nil {
		rep.Reason = perr.Error()
	} else {
		rep.Plan = &plan
	}
	if *apply && perr == nil {
		if err := hostswap.Apply(context.Background(), plan, hostswap.FstabPath); err != nil {
			return reportError(stdout, stderr, jsonOut, newValidationError("%v", err))
		}
		rep.Applied = true
	}
	if jsonOut || of.Format != outputTable {
		return emitJSON(stdout, stderr, rep)
	}
	printSwapReport(stdout, prog, rep, *apply)
	if perr != nil {
		return exitValidation
	}
	return exitOK
}

func printSwapReport(w io.Writer, prog string, rep swapReport, applyRequested bool) {
	_, _ = fmt.Fprintf(w, "Host: %s\n", rep.State.Summary())
	if rep.Plan == nil {
		_, _ = fmt.Fprintf(w, "No change planned: %s\n", rep.Reason)
		return
	}
	if rep.Applied {
		_, _ = fmt.Fprintf(w, "Created and enabled %d MiB of swap at %s.\n", rep.Plan.SizeMB, rep.Plan.Path)
		return
	}
	_, _ = fmt.Fprintln(w, "Dry run. These commands would run as root, nothing was changed:")
	for _, c := range rep.Plan.Commands {
		_, _ = fmt.Fprintf(w, "  %s\n", c)
	}
	for _, n := range rep.Plan.Notes {
		_, _ = fmt.Fprintf(w, "Note: %s\n", n)
	}
	if !applyRequested {
		_, _ = fmt.Fprintf(w, "Run 'sudo %s swap --apply' on this host to do it.\n", prog)
	}
}

// configuredSwapSizeMB prefers the live setting from the control plane, then the env seed, then 0 (auto).
func configuredSwapSizeMB(tokenFlag, apiURLFlag, profileFlag, prog string, lookupEnv func(string) (string, bool)) int {
	if doctorServerConfigured(tokenFlag, apiURLFlag, profileFlag, prog, lookupEnv) {
		client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
		if s, err := client.GetRuntimeSetting(context.Background(), keyHostSwapSizeMB); err == nil {
			if f, ok := s.Value.(float64); ok && f > 0 {
				return int(f)
			}
			return 0
		}
	}
	if v, ok := lookupEnv(envHostSwapSizeMB); ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 0
}

// localSwapCheck is the offline doctor row: RAM, swap and swappiness, with the helper as the fix.
func localSwapCheck(lookupEnv func(string) (string, bool)) doctorCheckResource {
	c := doctorCheckResource{Code: "host_swap", Name: "Swap", Group: localDoctorGroupHost}
	st := hostswap.Read("")
	if !st.Supported {
		c.Status, c.Message = "unknown", "this operating system does not report swap"
		return c
	}
	c.Evidence = st.Summary()
	limitMB := defaultNoSwapRAMMB
	if v, ok := lookupEnv(envHostNoSwapRAMMB); ok {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			limitMB = n
		}
	}
	if st.SwapBytes == 0 && st.RAMBytes>>20 < int64(limitMB) {
		c.Status, c.Message = "warn", "no swap on a low memory host; a memory spike can kill builds and apps"
		c.Fix = "the CLI command swap prints the exact commands (dry run), and swap --apply run as root creates the swapfile"
		return c
	}
	c.Status, c.Message = "ok", st.Summary()
	return c
}
