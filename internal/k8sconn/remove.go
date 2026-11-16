package k8sconn

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// RemoveResult lists what Remove deleted and what was already gone.
type RemoveResult struct {
	Deleted []string
	Absent  []string
}

// Remove deletes the Deployment, Service and Ingress of one app, and nothing else. Every
// object must carry our managed-by, app and instance labels or the call refuses before the
// first delete. A missing object counts as success so a retry is safe. The namespace stays.
func (c *Client) Remove(ctx context.Context, namespace, app, instance string) (RemoveResult, error) {
	ns := url.PathEscape(namespace)
	name := url.PathEscape(app)
	refs := []ObjectRef{
		{Kind: "Deployment", Name: app, Path: "/apis/apps/v1/namespaces/" + ns + "/deployments"},
		{Kind: "Service", Name: app, Path: "/api/v1/namespaces/" + ns + "/services"},
		{Kind: "Ingress", Name: app, Path: "/apis/networking.k8s.io/v1/namespaces/" + ns + "/ingresses"},
	}
	var res RemoveResult
	var present []ObjectRef
	for _, ref := range refs {
		labels, found, err := c.getLabels(ctx, ref.Path+"/"+name)
		if err != nil {
			return res, fmt.Errorf("%s %s: %w", ref.Kind, ref.Name, err)
		}
		if !found {
			res.Absent = append(res.Absent, ref.Kind+"/"+ref.Name)
			continue
		}
		// An instance is required here: an empty one would match another install's objects.
		if instance == "" || !ownedBy(labels, app, instance) {
			return res, fmt.Errorf("%s %s: %w", ref.Kind, ref.Name, ErrNotManaged)
		}
		present = append(present, ref)
	}
	for _, ref := range present {
		_, err := c.Do(ctx, http.MethodDelete, ref.Path+"/"+name, nil)
		if isStatus(err, http.StatusNotFound) {
			res.Absent = append(res.Absent, ref.Kind+"/"+ref.Name)
			continue
		}
		if err != nil {
			return res, fmt.Errorf("%s %s: %w", ref.Kind, ref.Name, err)
		}
		res.Deleted = append(res.Deleted, ref.Kind+"/"+ref.Name)
	}
	return res, nil
}
