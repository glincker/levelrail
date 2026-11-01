package application

import (
	"strconv"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

const reasonReadOnlyRootfsDriftRecreate = "ReadOnlyRootfsDriftRecreateFailed"

func desiredReadOnlyRootfs(desired *store.DesiredService) *bool {
	if desired.Resources == nil {
		return nil
	}
	return desired.Resources.ReadOnlyRootfs
}

// readOnlyRootfsSpec stamps an explicit choice as a reserved label (replacing
// any user copy) so drift shows in a container listing and a remote agent
// applies it. Inherit leaves no label.
func readOnlyRootfsSpec(desired *store.DesiredService, labels map[string]string) map[string]string {
	want := desiredReadOnlyRootfs(desired)
	if _, had := labels[docker.ReadOnlyRootfsLabelKey]; want == nil && !had {
		return labels
	}
	out := make(map[string]string, len(labels)+1)
	for k, v := range labels {
		out[k] = v
	}
	delete(out, docker.ReadOnlyRootfsLabelKey)
	if want != nil {
		out[docker.ReadOnlyRootfsLabelKey] = strconv.FormatBool(*want)
	}
	return out
}

// readOnlyRootfsDrifted reports a container created with a different explicit
// choice. The root mode cannot change in place, so convergence is a recreate.
func readOnlyRootfsDrifted(state *docker.ContainerState, desired *store.DesiredService) bool {
	if state == nil || state.LabelsUnknown {
		return false
	}
	have, haveSet := docker.ReadOnlyRootfsFromLabels(state.Labels)
	want := desiredReadOnlyRootfs(desired)
	if want == nil {
		return haveSet
	}
	return !haveSet || have != *want
}
