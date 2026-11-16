package k8sconn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

// FieldManager owns every field Levelrail applies.
const FieldManager = "levelrail"

// ErrNotManaged means an object with this name exists but lacks our labels.
var ErrNotManaged = errors.New("k8sconn: an object with this name exists and is not managed by levelrail")

// ObjectRef names one namespaced or cluster scoped object by its API path.
type ObjectRef struct {
	Kind string
	Name string
	Path string // collection path, without the name
}

// Refs returns the API paths of every rendered object, namespace first.
func (r Rendered) Refs(namespace string) []ObjectRef {
	refs := []ObjectRef{
		{Kind: "Deployment", Name: nameOf(r.Deployment), Path: "/apis/apps/v1/namespaces/" + namespace + "/deployments"},
		{Kind: "Service", Name: nameOf(r.Service), Path: "/api/v1/namespaces/" + namespace + "/services"},
	}
	if r.Ingress != nil {
		refs = append(refs, ObjectRef{Kind: "Ingress", Name: nameOf(r.Ingress), Path: "/apis/networking.k8s.io/v1/namespaces/" + namespace + "/ingresses"})
	}
	return refs
}

func nameOf(obj map[string]any) string {
	meta, _ := obj["metadata"].(map[string]any)
	n, _ := meta["name"].(string)
	return n
}

type objectMeta struct {
	Metadata struct {
		Labels map[string]string `json:"labels"`
	} `json:"metadata"`
}

// getLabels returns the labels of an existing object, or found=false on 404.
func (c *Client) getLabels(ctx context.Context, path string) (labels map[string]string, found bool, err error) {
	data, err := c.Do(ctx, http.MethodGet, path, nil)
	var se *StatusError
	if errors.As(err, &se) && se.Code == http.StatusNotFound {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var m objectMeta
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, false, fmt.Errorf("k8sconn: decode object: %w", err)
	}
	return m.Metadata.Labels, true, nil
}

func ownedBy(labels map[string]string, app string) bool {
	return labels[LabelManagedBy] == ManagedByValue && labels[LabelApp] == app
}

func (c *Client) patch(ctx context.Context, path string, obj map[string]any, force bool) error {
	body, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	q := "?fieldManager=" + FieldManager
	if force {
		q += "&force=true"
	}
	_, err = c.doType(ctx, http.MethodPatch, path+q, "application/apply-patch+yaml", body)
	return err
}

// EnsureNamespace creates the namespace with our label when it is missing and never
// patches one that already exists.
func (c *Client) EnsureNamespace(ctx context.Context, name string) error {
	_, found, err := c.getLabels(ctx, "/api/v1/namespaces/"+url.PathEscape(name))
	if err != nil || found {
		return err
	}
	ns := map[string]any{
		"apiVersion": "v1", "kind": "Namespace",
		"metadata": map[string]any{"name": name, "labels": map[string]any{LabelManagedBy: ManagedByValue}},
	}
	return c.patch(ctx, "/api/v1/namespaces/"+url.PathEscape(name), ns, false)
}

// Apply checks every object first, so one foreign object stops the whole deploy before
// anything changes. Force is sent only for objects that already carry our labels.
func (c *Client) Apply(ctx context.Context, spec AppSpec, r Rendered) ([]string, error) {
	if err := c.EnsureNamespace(ctx, spec.Namespace); err != nil {
		return nil, err
	}
	type step struct {
		ref   ObjectRef
		obj   map[string]any
		path  string
		force bool
	}
	var steps []step
	objs := []map[string]any{r.Deployment, r.Service}
	if r.Ingress != nil {
		objs = append(objs, r.Ingress)
	}
	for i, ref := range r.Refs(spec.Namespace) {
		path := ref.Path + "/" + url.PathEscape(ref.Name)
		labels, found, err := c.getLabels(ctx, path)
		if err != nil {
			return nil, err
		}
		if found && !ownedBy(labels, spec.Name) {
			return nil, fmt.Errorf("%s %s: %w", ref.Kind, ref.Name, ErrNotManaged)
		}
		steps = append(steps, step{ref: ref, obj: objs[i], path: path, force: found})
	}
	names := []string{}
	for _, st := range steps {
		if err := c.patch(ctx, st.path, st.obj, st.force); err != nil {
			return names, fmt.Errorf("%s %s: %w", st.ref.Kind, st.ref.Name, err)
		}
		names = append(names, st.ref.Kind+"/"+st.ref.Name)
	}
	return names, nil
}
