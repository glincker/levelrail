package api

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"

	"github.com/glincker/levelrail/internal/catalog"
)

// kernelModuleLoaded is a variable so tests can fake the host kernel.
var kernelModuleLoaded = hostKernelModuleLoaded

// hostKernelModuleLoaded reports whether a module is loaded or built in. known is false
// when the host is not Linux or /proc is unreadable, so callers skip instead of refusing.
func hostKernelModuleLoaded(name string) (loaded, known bool) {
	if runtime.GOOS != "linux" {
		return false, false
	}
	if _, err := os.Stat("/sys/module/" + name); err == nil {
		return true, true
	}
	raw, err := os.ReadFile("/proc/modules")
	if err != nil {
		return false, false
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if f := strings.Fields(line); len(f) > 0 && f[0] == name {
			return true, true
		}
	}
	return false, true
}

// kernelModulePreflight refuses an install whose template needs a host kernel module that is
// not loaded. Remote nodes are not probed: it covers the control plane host only.
func kernelModulePreflight(tpl catalog.Template) string {
	if tpl.Manifest == nil {
		return ""
	}
	for _, mod := range tpl.Manifest.RequiresKernelModules {
		if loaded, known := kernelModuleLoaded(mod); known && !loaded {
			return fmt.Sprintf("template %q needs the %s kernel module and it is not loaded on this host (kernel module missing). Run `modprobe %s` on the host, make it load at boot, then retry. Levelrail never gives containers SYS_MODULE.", tpl.ID, mod, mod)
		}
	}
	return ""
}

// doctorCheckKernelModules reports each kernel module a catalog template needs. It checks
// the control plane host only; remote nodes are skipped.
func (rt *Router) doctorCheckKernelModules(_ context.Context) []doctorCheckResource {
	needed := map[string][]string{}
	for _, t := range catalog.Templates {
		if t.Manifest == nil {
			continue
		}
		for _, mod := range t.Manifest.RequiresKernelModules {
			needed[mod] = append(needed[mod], t.ID)
		}
	}
	mods := make([]string, 0, len(needed))
	for m := range needed {
		mods = append(mods, m)
	}
	sort.Strings(mods)
	var out []doctorCheckResource
	for _, mod := range mods {
		loaded, known := kernelModuleLoaded(mod)
		if !known {
			continue
		}
		c := doctorCheckResource{Code: "kernel_module:" + mod, Name: "Kernel module " + mod + " (control plane host)"}
		if loaded {
			c.Status = doctorStatusOK
			c.Message = mod + " is loaded. Remote nodes are not checked."
		} else {
			c.Status = doctorStatusWarn
			c.Message = mod + " is not loaded, so " + strings.Join(needed[mod], ", ") + " will refuse to install here."
			c.Fix = "Run `modprobe " + mod + "` on the host and add it to /etc/modules-load.d so it loads at boot."
		}
		out = append(out, c)
	}
	return out
}
