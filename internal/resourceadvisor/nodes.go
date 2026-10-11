package resourceadvisor

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/rightsizing"
)

func nodeRef(n Node) Ref { return Ref{Kind: ResourceNode, Name: n.ID} }

func nodeLabel(n Node) string {
	if n.Name != "" {
		return n.Name
	}
	return n.ID
}

// consolidateFinding is rule g: many small databases of one engine on a node.
func consolidateFinding(n Node, dbs []Resource, th Thresholds, now time.Time) []Finding {
	byEngine := map[string][]Resource{}
	for _, r := range dbs {
		mem := within(r.Memory, now.Add(-th.Window))
		if coverage(mem, now) < th.MinData || r.Suspended {
			continue
		}
		if int64(summarize(mem).p95) <= th.SmallDatabaseBytes {
			byEngine[r.Engine] = append(byEngine[r.Engine], r)
		}
	}
	engines := make([]string, 0, len(byEngine))
	for e := range byEngine {
		engines = append(engines, e)
	}
	sort.Strings(engines)
	var out []Finding
	for _, engine := range engines {
		group := byEngine[engine]
		if len(group) < th.SmallDatabaseCount {
			continue
		}
		var total, smallest float64
		names := make([]string, 0, len(group))
		for i, r := range group {
			p50 := summarize(within(r.Memory, now.Add(-th.Window))).p50
			total += p50
			if i == 0 || p50 < smallest {
				smallest = p50
			}
			names = append(names, r.Ref.Name)
		}
		sort.Strings(names)
		f := newFinding(KindConsolidate, SeverityInfo, nodeRef(n))
		f.ID = string(KindConsolidate) + ":" + nodeRef(n).String() + ":" + engine
		saving := int64(smallest) * int64(len(group)-1)
		f.Title = fmt.Sprintf("%d small %s databases run as separate instances on %s", len(group), engine, nodeLabel(n))
		f.Why = fmt.Sprintf("%s each run their own %s server, together about %s. Every instance carries its own buffers and background processes, so one instance with a database and role per app would likely save around %s (a rough estimate: the smallest footprint times %d).",
			strings.Join(names, ", "), engine, rightsizing.FormatMemory(int64(total)), rightsizing.FormatMemory(saving), len(group)-1)
		f.Evidence = Evidence{Window: rightsizing.HumanWindow(th.Window), WindowSeconds: int64(th.Window.Seconds()), Samples: len(group), Unit: UnitBytes, Numbers: map[string]float64{"count": float64(len(group)), "total_p50_bytes": total}}
		f.Impact.SavingMemoryBytes = saving
		f.Impact.Risk = "One shared instance means one failure domain, one engine version and extension set for all, noisy neighbours, and backups and restores that cover every database at once."
		f.Action = &Action{Type: ActionManual, Instructions: "Never done automatically. If the tradeoffs are acceptable, create one database, migrate each app's data into its own database and role there with the migration hub, switch the app, then stop the old instance."}
		f.Params["count"], f.Params["engine"], f.Params["node"], f.Params["names"] = fmt.Sprint(len(group)), engine, nodeLabel(n), strings.Join(names, ", ")
		f.Params["total"], f.Params["saving"] = rightsizing.FormatMemory(int64(total)), rightsizing.FormatMemory(saving)
		out = append(out, f)
	}
	return out
}

// diskFindings is rule h: disk fill forecast and reclaimable Docker data.
func diskFindings(n Node, th Thresholds, now time.Time) []Finding {
	var out []Finding
	used := within(n.DiskUsed, now.Add(-th.Window))
	if n.DiskTotal > 0 && coverage(used, now) >= th.MinData {
		if ft, ok := linearFit(buckets(used, time.Hour, false)); ok && ft.slopePerHour > 0 {
			current := ft.at(ft.end)
			left := time.Duration((n.DiskTotal - current) / ft.slopePerHour * float64(time.Hour))
			if left > 0 && left <= th.DiskHorizon {
				f := newFinding(KindDiskForecast, SeveritySuggest, nodeRef(n))
				if left < 7*day {
					f.Severity = SeverityWarn
				}
				eta := now.Add(left)
				f.Title = fmt.Sprintf("Disk on %s fills in about %s", nodeLabel(n), rightsizing.HumanDuration(left))
				f.Why = fmt.Sprintf("Disk use grew about %s a day over the %s and is at %.0f%% now. At this rate it is full around %s.",
					rightsizing.FormatMemory(int64(ft.slopePerHour*24)), rightsizing.HumanWindow(th.Window), current/n.DiskTotal*100, eta.UTC().Format("2006-01-02"))
				f.Evidence = memoryEvidence(used, int64(n.DiskTotal), th.Window)
				f.Impact.ExhaustionAt = &eta
				f.Impact.Risk = "A full disk stops databases from writing and Docker from starting containers."
				f.Action = &Action{Type: ActionManual, Instructions: "Prune unused images and build cache, check backup retention and log volume, or grow the disk."}
				f.Params["node"], f.Params["eta"] = nodeLabel(n), rightsizing.HumanDuration(left)
				out = append(out, f)
			}
		}
	}
	if n.Reclaimable != nil {
		total := n.Reclaimable.ImagesBytes + n.Reclaimable.BuildCacheBytes
		if total >= th.ReclaimableMin {
			f := newFinding(KindReclaimableDisk, SeveritySuggest, nodeRef(n))
			f.Title = fmt.Sprintf("%s of unused images and build cache on %s", rightsizing.FormatMemory(total), nodeLabel(n))
			f.Why = fmt.Sprintf("Docker reports %s in images no container uses and %s of build cache. Pruning frees it; the next build is slower until the cache refills.",
				rightsizing.FormatMemory(n.Reclaimable.ImagesBytes), rightsizing.FormatMemory(n.Reclaimable.BuildCacheBytes))
			f.Evidence = Evidence{Numbers: map[string]float64{"images_bytes": float64(n.Reclaimable.ImagesBytes), "build_cache_bytes": float64(n.Reclaimable.BuildCacheBytes)}, Unit: UnitBytes}
			f.Impact.SavingDiskBytes = total
			f.Action = &Action{Type: ActionPruneDocker, Instructions: "Use Clean up Docker from the status menu, or the CLI system-prune command. Docker counts every image no container uses, while prune removes dangling images and build cache, so it can free less than this."}
			f.Params["node"], f.Params["total"] = nodeLabel(n), rightsizing.FormatMemory(total)
			out = append(out, f)
		}
	}
	return out
}

// hostFindings is rule i: pressure, swap, top consumers and missing capacity.
func hostFindings(n Node, res []Resource, th Thresholds, now time.Time) []Finding {
	var out []Finding
	if n.MemoryTotal <= 0 || n.NoIdentity {
		f := newFinding(KindCapacityUnknown, SeverityInfo, nodeRef(n))
		f.Title = fmt.Sprintf("Memory capacity is not reported for %s", nodeLabel(n))
		f.Why = "Without host memory the dashboard cannot compare reserved limits with capacity, and the advisor cannot tell how close the server is to running out."
		f.Action = &Action{Type: ActionManual, Instructions: "Host memory is recorded per node only for the control plane's own node once it has a node identity (APP_MESH_ENABLED=1). Remote nodes do not report it yet."}
		if n.NoIdentity {
			f.Why = "This server has no node record (mesh is off), so host memory is not recorded per node and the dashboard shows Capacity not reported. The advisor reads host memory directly for its own checks."
		}
		f.Params["node"] = nodeLabel(n)
		out = append(out, f)
	}
	recent := now.Add(-time.Hour)
	if n.MemoryTotal > 0 {
		if avail := summarize(within(n.MemoryAvailable, recent)); avail.n > 0 {
			used := 1 - avail.mean/n.MemoryTotal
			if used >= th.HostPressure {
				f := newFinding(KindHostMemoryPressure, SeverityWarn, nodeRef(n))
				f.Title = fmt.Sprintf("Memory is running short on %s", nodeLabel(n))
				f.Why = fmt.Sprintf("Over the last hour %.0f%% of %s was in use on average. The kernel will start reclaiming cache and then OOM-killing containers.", used*100, rightsizing.FormatMemory(int64(n.MemoryTotal)))
				f.Evidence = Evidence{Window: "last 1 hour", WindowSeconds: 3600, Samples: avail.n, Unit: UnitBytes, Limit: n.MemoryTotal, Numbers: map[string]float64{"used_fraction": used}}
				f.Impact.Risk = "Under pressure any container without a limit can be the one the kernel kills."
				f.Action = &Action{Type: ActionManual, Instructions: "Set memory limits on the largest consumers, lower over-provisioned ones, or move work to another node."}
				f.Params["node"], f.Params["percent"] = nodeLabel(n), fmt.Sprintf("%.0f", used*100)
				out = append(out, f)
			}
		}
	}
	if swap := summarize(within(n.SwapUsed, recent)); swap.n > 0 && int64(swap.mean) >= th.SwapWarnBytes {
		f := newFinding(KindSwapInUse, SeveritySuggest, nodeRef(n))
		f.Title = fmt.Sprintf("%s of swap in use on %s", rightsizing.FormatMemory(int64(swap.mean)), nodeLabel(n))
		f.Why = "Swapping means memory demand exceeded RAM at some point; swapped database pages turn into slow disk reads."
		f.Evidence = Evidence{Window: "last 1 hour", WindowSeconds: 3600, Samples: swap.n, Unit: UnitBytes, Numbers: map[string]float64{"swap_bytes": swap.mean}}
		f.Action = &Action{Type: ActionManual, Instructions: "Find the largest consumers below and cap or move them."}
		f.Params["node"], f.Params["swap"] = nodeLabel(n), rightsizing.FormatMemory(int64(swap.mean))
		out = append(out, f)
	}
	if f, ok := topConsumersFinding(n, res, th, now); ok {
		out = append(out, f)
	}
	return out
}

func topConsumersFinding(n Node, res []Resource, th Thresholds, now time.Time) (Finding, bool) {
	type use struct {
		name  string
		bytes float64
	}
	var all []use
	var total float64
	for _, r := range res {
		cur := summarize(within(r.Memory, now.Add(-time.Hour)))
		if cur.n == 0 {
			continue
		}
		all = append(all, use{r.Ref.Name, cur.mean})
		total += cur.mean
	}
	if len(all) <= 3 || total <= 0 {
		return Finding{}, false
	}
	sort.Slice(all, func(i, j int) bool { return all[i].bytes > all[j].bytes })
	top := all[:3]
	var topSum float64
	names := make([]string, len(top))
	for i, u := range top {
		topSum += u.bytes
		names[i] = u.name
	}
	share := topSum / total
	// An even spread already gives three of n containers 3/n of the total.
	if share < th.TopConsumerShare || share < 1.5*3/float64(len(all)) {
		return Finding{}, false
	}
	f := newFinding(KindTopConsumers, SeverityInfo, nodeRef(n))
	f.Title = fmt.Sprintf("%s use %.0f%% of container memory on %s", strings.Join(names, ", "), share*100, nodeLabel(n))
	f.Why = fmt.Sprintf("Of %s used by %d containers, these three hold %s. They are where limits and tuning pay off first.",
		rightsizing.FormatMemory(int64(total)), len(all), rightsizing.FormatMemory(int64(topSum)))
	f.Evidence = Evidence{Window: "last 1 hour", WindowSeconds: 3600, Samples: len(all), Unit: UnitBytes, Numbers: map[string]float64{"share": share, "total_bytes": total}}
	if n.MemoryTotal > 0 {
		f.Evidence.Numbers["host_share"] = topSum / n.MemoryTotal
	}
	f.Params["names"], f.Params["percent"], f.Params["node"] = strings.Join(names, ", "), fmt.Sprintf("%.0f", share*100), nodeLabel(n)
	return f, true
}
