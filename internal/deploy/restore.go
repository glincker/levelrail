package deploy

import (
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// snapshotRecorded reports whether snap holds a real config capture; very
// old attempts predate snapshots and restore nothing.
func snapshotRecorded(snap store.DeployAttemptSnapshot) bool {
	return len(snap.Env) > 0 || snap.Port != 0 || snap.Health != nil || snap.Replicas != 0 || snap.Strategy != "" || snap.Resources != nil
}

// RestoreSnapshot restores snap's env literals, port, health, replicas,
// strategy, resources and labels onto current. Secret references, domains,
// volumes and host ports stay current: rolling them back can cut traffic or data.
func RestoreSnapshot(current store.DesiredService, snap store.DeployAttemptSnapshot) store.DesiredService {
	if !snapshotRecorded(snap) {
		return current
	}
	out := current
	inSnap := make(map[string]bool, len(snap.Env))
	env := make(map[string]string, len(current.Env))
	for _, k := range snap.Env {
		inSnap[k.Key] = true
		if k.Kind == store.DeployAttemptEnvKindLiteral {
			env[k.Key] = k.Value
		}
	}
	for k, v := range current.Env {
		if _, set := env[k]; set {
			continue
		}
		if inSnap[k] {
			env[k] = v
		}
	}
	out.Env = env
	out.EnvDirty = false
	if snap.Port != 0 {
		out.Port = snap.Port
	}
	// Snapshots taken before run-to-completion was recorded lack Restart.
	out.Health = store.KeepComposeHealth(snap.Health, current.Health)
	if snap.Replicas > 0 {
		out.Replicas = snap.Replicas
	}
	if snap.Strategy != "" {
		out.Strategy = snap.Strategy
	}
	out.Resources = snap.Resources
	if snap.Labels != nil {
		out.Labels = snap.Labels
	}
	return out
}

// RollbackDesired is current restored to target's config, plus the image
// reference to deploy it with. A pinned local tag never exists in any
// registry, so an always-pull policy is cleared when the pin is used.
func RollbackDesired(current store.DesiredService, target store.DeployAttempt, pin *store.RollbackPin) (svc store.DesiredService, ref string, contentSafe bool) {
	svc = RestoreSnapshot(current, target.Snapshot)
	ref, contentSafe = RollbackRef(target, pin)
	if LocalOnlyRef(pin, ref) {
		svc.PullPolicy = ""
	}
	return svc, ref, contentSafe
}

// RollbackRef is the image reference that redeploys exactly target's
// content: its registry digest when it had one, else its rollback pin,
// else the bare tag (the caller must verify a bare tag still matches).
func RollbackRef(target store.DeployAttempt, pin *store.RollbackPin) (ref string, contentSafe bool) {
	switch {
	case target.ImageDigest != "" && (target.DigestReason == store.DigestReasonResolved || target.DigestReason == store.DigestReasonPinned):
		return docker.PinImageRef(docker.UnpinImageRef(target.Image), target.ImageDigest), true
	case pin != nil && pin.PinRef != "":
		return pin.PinRef, true
	default:
		return target.Image, false
	}
}

// LocalOnlyRef reports whether ref is a rollback pin tag, which exists in
// no registry and so must never be pulled.
func LocalOnlyRef(pin *store.RollbackPin, ref string) bool {
	return pin != nil && pin.PinRef != "" && ref == pin.PinRef
}
