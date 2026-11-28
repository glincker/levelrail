package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"

	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// admitNewServiceName is the one gate every path runs before it writes a
// secret under a service name. An existing service passes (callers verify
// ownership); a new name must not be reserved, a database, a stack or parked.
func (rt *Router) admitNewServiceName(ctx context.Context, name string) error {
	if err := spec.AppNamespace(name); err != nil {
		return err
	}
	if _, err := rt.apps.GetDesiredService(ctx, name); err == nil {
		return nil
	} else if !errors.Is(err, store.ErrServiceNotFound) {
		return fmt.Errorf("check app %q: %w", name, err)
	}
	return rt.appNameFree(ctx, name)
}

// AdmitNewServiceName lets the deploy pipeline run the same gate before it stores file secrets.
func (rt *Router) AdmitNewServiceName(ctx context.Context, name string) error {
	return rt.admitNewServiceName(ctx, name)
}

// admittedNames is the set of names a request admitted and still holds locked.
type admittedNames struct {
	release func()
	fresh   []string
}

// admitServiceNames locks the names (sorted, so two requests cannot deadlock) and admits each;
// fresh lists the names with no service yet, whose secrets must be dropped if the save never happens.
func (rt *Router) admitServiceNames(w http.ResponseWriter, r *http.Request, names []string) (*admittedNames, bool) {
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	var unlocks []func()
	release := func() {
		for i := len(unlocks) - 1; i >= 0; i-- {
			unlocks[i]()
		}
	}
	adm := &admittedNames{release: release}
	prev := ""
	for _, n := range sorted {
		if n == prev {
			continue
		}
		prev = n
		unlocks = append(unlocks, lockAppName(n))
		if err := rt.admitNewServiceName(r.Context(), n); err != nil {
			release()
			switch {
			case errors.Is(err, spec.ErrReservedNamespace):
				writeError(w, http.StatusBadRequest, "service name "+n+" is reserved")
			case errors.Is(err, errAppNameTaken):
				writeError(w, http.StatusConflict, "an app named "+n+" already exists")
			default:
				rt.internalError(w, "api: admit service name failed", err)
			}
			return nil, false
		}
		if _, err := rt.apps.GetDesiredService(r.Context(), n); errors.Is(err, store.ErrServiceNotFound) {
			adm.fresh = append(adm.fresh, n)
		}
	}
	return adm, true
}

type writtenSecret struct{ service, key string }

// dropUnsavedSecrets removes secrets this call wrote for names that still have no service row.
func (rt *Router) dropUnsavedSecrets(ctx context.Context, fresh []string, written []writtenSecret) {
	isFresh := make(map[string]bool, len(fresh))
	for _, n := range fresh {
		isFresh[n] = true
	}
	deleter, ok := rt.secrets.(interface {
		DeleteValue(ctx context.Context, serviceName, envKey string) error
	})
	if !ok {
		return
	}
	ctx = context.WithoutCancel(ctx)
	for _, s := range written {
		if !isFresh[s.service] {
			continue
		}
		if _, err := rt.apps.GetDesiredService(ctx, s.service); !errors.Is(err, store.ErrServiceNotFound) {
			continue
		}
		if err := deleter.DeleteValue(ctx, s.service, s.key); err != nil {
			rt.logger.Warn("api: drop unsaved secret failed", "service", s.service, "key", s.key, "error", err.Error())
		}
	}
}

// previewShapedName matches "<x>-pr-<n>" and "<x>-pr-<n>-<key>", the names only the preview system may create.
var previewShapedName = regexp.MustCompile(`^.+-pr-[0-9]+(-.+)?$`)

type previewSystemKey struct{}

// withPreviewSystem marks a context so appNameFree accepts preview-shaped names.
func withPreviewSystem(ctx context.Context) context.Context {
	return context.WithValue(ctx, previewSystemKey{}, true)
}

func previewShapedNameRefused(ctx context.Context, name string) bool {
	if v, _ := ctx.Value(previewSystemKey{}).(bool); v {
		return false
	}
	return previewShapedName.MatchString(name)
}

// lockAdmitPreviewServices locks and admits every service name a multi-service preview will deploy.
// A name that is already a member of the preview app passes; any other existing name belongs to someone else.
func (rt *Router) lockAdmitPreviewServices(ctx context.Context, previewName string, keys []string) (func(), error) {
	names := make([]string, 0, len(keys))
	for _, k := range keys {
		names = append(names, previewName+"-"+k)
	}
	sort.Strings(names)
	members := map[string]bool{}
	if existing, err := rt.appGroups.ListServicesByApp(ctx, previewName); err == nil {
		for _, m := range existing {
			members[m.Name] = true
		}
	}
	var unlocks []func()
	release := func() {
		for i := len(unlocks) - 1; i >= 0; i-- {
			unlocks[i]()
		}
	}
	for _, n := range names {
		unlocks = append(unlocks, lockAppName(n))
		if members[n] {
			continue
		}
		if err := rt.appNameFree(withPreviewSystem(ctx), n); err != nil {
			release()
			return nil, fmt.Errorf("preview service name %q: %w", n, err)
		}
	}
	return release, nil
}
