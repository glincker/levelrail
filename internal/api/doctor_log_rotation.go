package api

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/docker"
)

type containerLister interface {
	ListByPrefix(ctx context.Context, prefix string) ([]docker.ContainerState, error)
}

// doctorCheckLogRotation inspects running containers one by one, up to a cap, and names those whose
// json-file log has no size limit. It only reads; the fix is a setting for new containers.
func (rt *Router) doctorCheckLogRotation(ctx context.Context) doctorCheckResource {
	const code, name = "container_log_rotation", "Container log rotation"
	lister, okL := rt.dockerPinger.(containerLister)
	insp, okI := rt.dockerPinger.(docker.LogRotationInspector)
	if !okL || !okI {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "container log settings are not available from this client"}
	}
	all, err := lister.ListByPrefix(ctx, "")
	if err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "could not list containers"}
	}
	limit := int(envFloat("APP_DOCTOR_LOG_ROTATION_MAX_INSPECT", 50))
	var unrotated []string
	checked, running := 0, 0
	for _, c := range all {
		if !c.Running {
			continue
		}
		running++
		if checked >= limit {
			continue
		}
		info, err := insp.InspectLogRotation(ctx, c.ID)
		if err != nil {
			continue
		}
		checked++
		if logUnbounded(info) {
			unrotated = append(unrotated, c.Name)
		}
	}
	if running == 0 {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: "no running containers"}
	}
	scope := fmt.Sprintf("%d of %d running containers checked", checked, running)
	if len(unrotated) == 0 {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: scope + ", none keep an unbounded log"}
	}
	sort.Strings(unrotated)
	shown := unrotated
	if len(shown) > 5 {
		shown = shown[:5]
	}
	return doctorCheckResource{
		Code: code, Name: name, Status: doctorStatusWarn,
		Message: fmt.Sprintf("%d containers keep a json-file log with no size limit (%s), %s", len(unrotated), strings.Join(shown, ", "), scope),
		Fix:     "Set containers.log_max_size_mb and containers.log_max_files in runtime settings, then redeploy the apps so new containers pick them up.",
	}
}

// logUnbounded is true only for the json-file driver with no max-size; other drivers manage their own retention.
func logUnbounded(i docker.LogRotationInfo) bool {
	return (i.Driver == "json-file" || i.Driver == "") && i.MaxSize == ""
}
