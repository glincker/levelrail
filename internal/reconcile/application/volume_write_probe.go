package application

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/GLINCKER/levelrail/internal/deploytrace"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// EnvVolumeWriteProbe set to 0, false or off turns the post-start volume write check off.
const EnvVolumeWriteProbe = "APP_VOLUME_WRITE_PROBE"

// ConditionTypeVolumesWritable reports whether the app's user can write its volume roots.
const ConditionTypeVolumesWritable = "VolumesWritable"

// Reasons on ConditionTypeVolumesWritable.
const (
	ReasonVolumesWritable   = "Writable"
	ReasonVolumeNotWritable = "VolumeNotWritable"
	ReasonVolumesUnverified = "Unverified"
)

func volumeWriteProbeOff() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(EnvVolumeWriteProbe)))
	return v == "0" || v == "false" || v == "off"
}

// volumeWriteMemo keeps the last check per container, so the result is
// reported on every pass while the probe itself runs once per start.
type volumeWriteMemo struct {
	mu          sync.Mutex
	byContainer map[string]reconcile.Condition
}

func (m *volumeWriteMemo) set(container string, cond reconcile.Condition) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.byContainer == nil {
		m.byContainer = map[string]reconcile.Condition{}
	}
	m.byContainer[container] = cond
}

// worst drops entries for containers no longer targeted and returns the most
// severe result among the rest: False, then Unknown, then True.
func (m *volumeWriteMemo) worst(targets []string) (reconcile.Condition, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	live := make(map[string]bool, len(targets))
	for _, t := range targets {
		live[t] = true
	}
	var out reconcile.Condition
	outName, found := "", false
	rank := map[reconcile.ConditionStatus]int{reconcile.ConditionTrue: 0, reconcile.ConditionUnknown: 1, reconcile.ConditionFalse: 2}
	for name, cond := range m.byContainer {
		if !live[name] {
			delete(m.byContainer, name)
			continue
		}
		r, best := rank[cond.Status], rank[out.Status]
		if !found || r > best || (r == best && name < outName) {
			out, outName, found = cond, name, true
		}
	}
	return out, found
}

// appendVolumeWriteCondition reports the volume write check when one ran for a current target.
func (c *Controller) appendVolumeWriteCondition(result reconcile.Result, targets []string) reconcile.Result {
	if cond, ok := c.volumeWrites.worst(targets); ok {
		result.Conditions = append(result.Conditions, cond)
	}
	return result
}

// probeVolumeWrites checks once, right after a start, that the container's
// user can write each writable volume root, and records it on the trace.
func (c *Controller) probeVolumeWrites(ctx context.Context, tr *deploytrace.Tracer, name, id string, volumes []docker.VolumeMount) {
	writable := make([]docker.VolumeMount, 0, len(volumes))
	for _, v := range volumes {
		if !v.ReadOnly {
			writable = append(writable, v)
		}
	}
	if len(writable) == 0 || volumeWriteProbeOff() {
		return
	}
	key := deploytrace.StepEnsureVolume + ".writable." + name
	prober, ok := c.runtime.(docker.VolumeAccessProber)
	if !ok {
		msg := "this node's runtime cannot check volume permissions, so writes by the app user are not verified"
		tr.NoteKey(ctx, deploytrace.StepEnsureVolume, key, store.TraceStatusSkipped, ReasonVolumesUnverified, msg)
		c.volumeWrites.set(name, volumeCondition(reconcile.ConditionUnknown, ReasonVolumesUnverified, msg))
		return
	}
	res, err := prober.ProbeVolumeAccess(ctx, id, writable)
	if err != nil {
		msg := "could not check volume permissions: " + err.Error()
		tr.NoteKey(ctx, deploytrace.StepEnsureVolume, key, store.TraceStatusSkipped, ReasonVolumesUnverified, msg)
		c.volumeWrites.set(name, volumeCondition(reconcile.ConditionUnknown, ReasonVolumesUnverified, msg))
		return
	}
	var bad []string
	for _, a := range res {
		if !a.Writable {
			bad = append(bad, volumeNotWritableText(a))
		}
	}
	if len(bad) == 0 {
		tr.NoteKey(ctx, deploytrace.StepEnsureVolume, key, store.TraceStatusDone, "", fmt.Sprintf("%d volume(s) writable by the app user", len(res)))
		c.volumeWrites.set(name, volumeCondition(reconcile.ConditionTrue, ReasonVolumesWritable, ""))
		return
	}
	msg := strings.Join(bad, "; ")
	tr.NoteKey(ctx, deploytrace.StepEnsureVolume, key, store.TraceStatusWarning, ReasonVolumeNotWritable, msg)
	c.volumeWrites.set(name, volumeCondition(reconcile.ConditionFalse, ReasonVolumeNotWritable, msg))
}

func volumeCondition(status reconcile.ConditionStatus, reason, msg string) reconcile.Condition {
	return reconcile.Condition{Type: ConditionTypeVolumesWritable, Status: status, Reason: reason, Message: msg}
}

// volumeNotWritableText names the path, the user and the owner, then the fix.
func volumeNotWritableText(a docker.VolumeAccess) string {
	who := strconv.Itoa(a.UID)
	if name, _, _ := strings.Cut(a.User, ":"); name != "" && name != who {
		who = fmt.Sprintf("%s (uid %d)", name, a.UID)
	}
	ids := fmt.Sprintf("%d:%d", a.UID, a.GID)
	return fmt.Sprintf("volume %s is not writable by user %s, its root is owned by %d:%d with mode %o: fix: chown the volume root to %s, or add RUN mkdir -p %s && chown %s %s before USER in the Dockerfile so a new volume starts with that owner",
		a.Path, who, a.OwnerUID, a.OwnerGID, a.Mode&0o7777, ids, a.Path, ids, a.Path)
}
