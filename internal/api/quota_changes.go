package api

import (
	"errors"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/quota"
	"github.com/GLINCKER/levelrail/internal/store"
)

// growth is how much more of each dimension next asks for than prev; a
// shrink counts as zero so lowering an over-quota resource is never refused.
func growth(prev, next quota.Amounts) quota.Amounts {
	return quota.Amounts{
		MemoryBytes: max(next.MemoryBytes-prev.MemoryBytes, 0),
		NanoCPUs:    max(next.NanoCPUs-prev.NanoCPUs, 0),
	}
}

// admitAppResizeQuota gates a resource or replica increase on an app.
func (rt *Router) admitAppResizeQuota(w http.ResponseWriter, r *http.Request, existing store.DesiredService, next store.DesiredService) (func(), bool) {
	delta := growth(serviceReservation(existing.Resources, existing.Replicas), serviceReservation(next.Resources, next.Replicas))
	if delta.Unlimited() {
		return func() {}, true
	}
	return rt.admitQuotaOrWrite(w, r, existing.ProjectID, store.ResourceOwner{}, delta)
}

// admitDatabaseResizeQuota gates a resource increase on a database.
func (rt *Router) admitDatabaseResizeQuota(w http.ResponseWriter, r *http.Request, existing store.DesiredDatabase, next *store.ServiceResources) (func(), bool) {
	delta := growth(serviceReservation(existing.Resources, 1), serviceReservation(next, 1))
	if delta.Unlimited() {
		return func() {}, true
	}
	return rt.admitQuotaOrWrite(w, r, existing.ProjectID, store.ResourceOwner{}, delta)
}

// admitAppMoveQuota gates moving an app into a project: the destination
// gains the app and its reservation. A missing app falls through so the
// handler's own 404 stays authoritative.
func (rt *Router) admitAppMoveQuota(w http.ResponseWriter, r *http.Request, name, projectID string) (func(), bool) {
	noop := func() {}
	db := rt.quotaDB()
	if db == nil {
		return noop, true
	}
	owner, err := db.OwnerOfApp(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: quota: owner of app", err)
		return nil, false
	}
	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) || owner == nil {
		return noop, true
	}
	if err != nil {
		rt.internalError(w, "api: quota: load app", err)
		return nil, false
	}
	res := serviceReservation(svc.Resources, svc.Replicas)
	return rt.admitQuotaOrWrite(w, r, projectID, *owner, quota.Amounts{Apps: 1, MemoryBytes: res.MemoryBytes, NanoCPUs: res.NanoCPUs})
}

// admitDatabaseMoveQuota is admitAppMoveQuota for a database.
func (rt *Router) admitDatabaseMoveQuota(w http.ResponseWriter, r *http.Request, name, projectID string) (func(), bool) {
	noop := func() {}
	db := rt.quotaDB()
	if db == nil {
		return noop, true
	}
	owner, err := db.OwnerOfDatabase(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: quota: owner of database", err)
		return nil, false
	}
	d, err := rt.databases.GetDesiredDatabase(r.Context(), name)
	if errors.Is(err, store.ErrDatabaseNotFound) || owner == nil {
		return noop, true
	}
	if err != nil {
		rt.internalError(w, "api: quota: load database", err)
		return nil, false
	}
	res := serviceReservation(d.Resources, 1)
	return rt.admitQuotaOrWrite(w, r, projectID, *owner, quota.Amounts{Databases: 1, MemoryBytes: res.MemoryBytes, NanoCPUs: res.NanoCPUs})
}
