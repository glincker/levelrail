package api

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/glincker/levelrail/internal/store"
)

const (
	errCodeAppVolumeForeign     = "app_volume_other_owner"
	errCodeAppVolumeUnknown     = "app_volume_owner_unknown"
	errCodeAppVolumeUnreachable = "app_volume_node_unreachable"
)

// appVolumeOwnerStore is the volume owner table plus the instance ids; test fakes need not implement it.
type appVolumeOwnerStore interface {
	ServiceInstanceID(ctx context.Context, name string) (string, error)
	GetAppVolumeInstance(ctx context.Context, volume string) (string, error)
	SetAppVolumeInstance(ctx context.Context, volume, instanceID string) error
	ListAppVolumeInstances(ctx context.Context) (map[string]string, error)
}

func (rt *Router) appVolumeOwners() appVolumeOwnerStore {
	s, _ := any(rt.apps).(appVolumeOwnerStore)
	return s
}

// volumeProbe says whether a volume exists on the app's node; known is false when it cannot say.
type volumeProbe func(ctx context.Context, volume string) (exists, known bool)

// appVolumeExists asks the app's node whether the volume is there; known is false when it cannot say.
func (rt *Router) appVolumeExists(ctx context.Context, nodeID, volume string) (exists, known bool) {
	if rt.backupProtection == nil || rt.execRuntime == nil {
		return false, false
	}
	nodeRuntime, err := rt.execRuntime(nodeID)
	if err != nil {
		return false, false
	}
	exists, known, err = rt.backupProtection.VolumeExists(ctx, nodeRuntime, volume)
	if err != nil {
		return false, false
	}
	return exists, known
}

// appVolumeReuseVerdict decides whether the app instance holding app may mount volumes that already
// exist on its node. appExisted is true for an app saved before this request: its unrecorded
// volumes are claimed (lazy backfill) instead of refused. It returns a stable code and message, or "".
func (rt *Router) appVolumeReuseVerdict(ctx context.Context, app string, volumes []store.ServiceVolume, appExisted bool, probe volumeProbe) (code, msg string) {
	os := rt.appVolumeOwners()
	if os == nil || len(volumes) == 0 {
		return "", ""
	}
	id, err := os.ServiceInstanceID(ctx, app)
	if err != nil {
		return errCodeAppVolumeUnknown, "could not read the owner of this app's volumes, retry"
	}
	for _, v := range volumes {
		if v.Name == "" {
			continue
		}
		owner, err := os.GetAppVolumeInstance(ctx, v.Name)
		if err != nil {
			return errCodeAppVolumeUnknown, "could not read the owner of volume " + v.Name + ", retry"
		}
		if owner == id {
			continue
		}
		exists, known := probe(ctx, v.Name)
		switch {
		case known && !exists, !known && owner == "":
			// Nothing to inherit, or no evidence either way: this instance becomes the owner.
		case !known:
			return errCodeAppVolumeUnreachable, fmt.Sprintf("volume %q belongs to another app instance and the node cannot confirm it is gone (agent offline or too old); bring the agent online or upgrade it, then retry", v.Name)
		case owner != "":
			return errCodeAppVolumeForeign, fmt.Sprintf("volume %q holds data of an earlier app with this name; choose another app name or remove the volume yourself after checking its data", v.Name)
		case !appExisted:
			return errCodeAppVolumeUnknown, fmt.Sprintf("volume %q already exists on the node and its owner was never recorded; choose another app name or remove the volume yourself after checking its data", v.Name)
		}
		if err := os.SetAppVolumeInstance(ctx, v.Name, id); err != nil {
			return errCodeAppVolumeUnknown, "could not record the owner of volume " + v.Name + ", retry"
		}
	}
	return "", ""
}

// refuseForeignAppVolumes writes the 409 and returns true when a volume cannot be reused.
func (rt *Router) refuseForeignAppVolumes(w http.ResponseWriter, r *http.Request, app, nodeID string, volumes []store.ServiceVolume, appExisted bool) bool {
	code, msg := rt.appVolumeReuseVerdict(r.Context(), app, volumes, appExisted, func(ctx context.Context, volume string) (bool, bool) {
		return rt.appVolumeExists(ctx, nodeID, volume)
	})
	if code == "" {
		return false
	}
	writeCodedError(w, http.StatusConflict, code, msg)
	return true
}

// doctorCheckAppVolumeOwners lists app volumes on this node that no live app owns or that another instance does.
func (rt *Router) doctorCheckAppVolumeOwners(ctx context.Context) doctorCheckResource {
	const code, name = "app_volume_owners", "App volume ownership"
	os := rt.appVolumeOwners()
	if os == nil || rt.orphanedVolumes == nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: "not tracked on this install"}
	}
	vols, err := rt.orphanedVolumes.ListNamedVolumes(ctx)
	if err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: fmt.Sprintf("could not list volumes: %s", err)}
	}
	owners, err := os.ListAppVolumeInstances(ctx)
	if err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: fmt.Sprintf("could not read volume owners: %s", err)}
	}
	services, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: fmt.Sprintf("could not list apps: %s", err)}
	}
	held := map[string]string{}
	for _, svc := range services {
		id, ierr := os.ServiceInstanceID(ctx, svc.Name)
		if ierr != nil {
			continue
		}
		for _, v := range svc.Volumes {
			held[v.Name] = id
		}
	}
	var foreign, kept []string
	for _, v := range vols {
		if !strings.HasPrefix(v.Name, "app-") {
			continue
		}
		id, live := held[v.Name]
		switch {
		case live && owners[v.Name] != "" && owners[v.Name] != id:
			foreign = append(foreign, v.Name)
		case !live && owners[v.Name] != "":
			kept = append(kept, v.Name)
		}
	}
	sort.Strings(foreign)
	sort.Strings(kept)
	if len(foreign) == 0 && len(kept) == 0 {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: "every app volume matches its owner"}
	}
	status := doctorStatusWarn
	return doctorCheckResource{
		Code: code, Name: name, Status: status,
		Message:  fmt.Sprintf("%d volume(s) belong to another app instance and %d are kept from deleted apps", len(foreign), len(kept)),
		Evidence: "foreign: " + strings.Join(foreign, ", ") + "; kept: " + strings.Join(kept, ", "),
		Fix:      "nothing is deleted for you; check the data, then remove a volume you no longer need from Storage",
	}
}
