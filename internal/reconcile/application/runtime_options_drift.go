package application

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

const reasonRuntimeOptionsDriftRecreate = "RuntimeOptionsDriftRecreateFailed"

func desiredRuntimeOptionsHash(desired *store.DesiredService) string {
	k := desired.Runtime.IdentityKey()
	// Granted caps join the identity so a grant or its revocation recreates the container.
	if len(desired.GrantedCaps) > 0 {
		k += "|caps:" + strings.Join(desired.GrantedCaps, ",")
	}
	if k == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(k))
	return hex.EncodeToString(sum[:8])
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
	return state.Labels[docker.RuntimeOptionsLabelKey] != desiredRuntimeOptionsHash(desired)
}
