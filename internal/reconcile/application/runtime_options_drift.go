package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

const reasonRuntimeOptionsDriftRecreate = "RuntimeOptionsDriftRecreateFailed"

// runtimeOptionsBaseKey is the identity before the io term, which is also the
// whole identity older releases stamped on containers.
func runtimeOptionsBaseKey(desired *store.DesiredService) string {
	k := desired.Runtime.IdentityKey()
	// Granted caps join the identity so a grant or its revocation recreates the container.
	if len(desired.GrantedCaps) > 0 {
		k += "|caps:" + strings.Join(desired.GrantedCaps, ",")
	}
	return k
}

func hashRuntimeOptionsKey(k string) string {
	if k == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(k))
	return hex.EncodeToString(sum[:8])
}

func desiredRuntimeOptionsHash(desired *store.DesiredService) string {
	k := runtimeOptionsBaseKey(desired)
	// Block IO caps are set at create and cannot change live, so a new cap recreates. The v2
	// marker only appears with a cap, so apps without caps keep the hash older releases stamped.
	if r := desired.Resources; r != nil && (r.IOReadBps > 0 || r.IOWriteBps > 0) {
		k += "|v2|io:" + strconv.FormatInt(r.IOReadBps, 10) + "," + strconv.FormatInt(r.IOWriteBps, 10)
	}
	return hashRuntimeOptionsKey(k)
}

// runtimeOptionsSpec stamps the option hash as a reserved label (replacing any user
// copy) so a changed option recreates only this container, under the same name.
func runtimeOptionsSpec(desired *store.DesiredService, labels map[string]string) map[string]string {
	want := desiredRuntimeOptionsHash(desired)
	if _, had := labels[docker.RuntimeOptionsLabelKey]; want == "" && !had {
		return labels
	}
	out := make(map[string]string, len(labels)+1)
	for k, v := range labels {
		out[k] = v
	}
	delete(out, docker.RuntimeOptionsLabelKey)
	if want != "" {
		out[docker.RuntimeOptionsLabelKey] = want
	}
	return out
}

// runtimeOptionsDrifted reports a container created with different options; they
// are create-time settings, so convergence is a recreate.
func runtimeOptionsDrifted(state *docker.ContainerState, desired *store.DesiredService) bool {
	if state == nil || state.LabelsUnknown {
		return false
	}
	got := state.Labels[docker.RuntimeOptionsLabelKey]
	if got == desiredRuntimeOptionsHash(desired) {
		return false
	}
	// A label from before io caps joined the hash counts as converged, so an upgrade does not
	// roll every labeled capped container. No label cannot be told from "never capped", so it recreates.
	return got == "" || got != hashRuntimeOptionsKey(runtimeOptionsBaseKey(desired))
}

// imageLocal guards a drift recreate: if the image is gone locally the recreate
// could fail to pull and leave an outage, so keep the running container. Unknown counts as present.
func (c *Controller) imageLocal(ctx context.Context, ref string) bool {
	insp, ok := c.runtime.(docker.ImageInspector)
	if !ok {
		return true
	}
	id, err := insp.InspectImageID(ctx, ref)
	return err != nil || id != ""
}
