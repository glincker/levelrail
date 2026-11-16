package k8sconn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// FieldManager owns every field Levelrail applies.
const FieldManager = "levelrail"

// ErrNotManaged means an object with this name exists but lacks our labels.
var ErrNotManaged = errors.New("k8sconn: an object with this name exists and is not managed by levelrail")

// ErrNamespaceNotManaged means the namespace exists without our label and is not allowed.
var ErrNamespaceNotManaged = errors.New("k8sconn: the namespace exists and is not managed by levelrail")

// ErrHostTaken means another Ingress we do not own already claims the host.
var ErrHostTaken = errors.New("k8sconn: the ingress host is already used by an ingress levelrail does not own")

// ErrHostUnverified means Ingresses could not be listed, so host ownership is unknown.
var ErrHostUnverified = errors.New("k8sconn: ingress hosts could not be listed, so the host cannot be verified")

// ObjectRef names one namespaced or cluster scoped object by its API path.
type ObjectRef struct {
	Kind string
	Name string
	Path string // collection path, without the name
}

// Refs returns the API paths of every rendered object, namespace first.
func (r Rendered) Refs(namespace string) []ObjectRef {
	var refs []ObjectRef
	if r.Secret != nil {
		// First, so the pods never start before their pull credential exists.
		refs = append(refs, ObjectRef{Kind: "Secret", Name: nameOf(r.Secret), Path: "/api/v1/namespaces/" + namespace + "/secrets"})
	}
	refs = append(refs,
		ObjectRef{Kind: "Deployment", Name: nameOf(r.Deployment), Path: "/apis/apps/v1/namespaces/" + namespace + "/deployments"},
		ObjectRef{Kind: "Service", Name: nameOf(r.Service), Path: "/api/v1/namespaces/" + namespace + "/services"},
	)
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

func ownedBy(labels map[string]string, app, instance string) bool {
	if labels[LabelManagedBy] != ManagedByValue || labels[LabelApp] != app {
		return false
	}
	return instance == "" || labels[LabelInstance] == instance
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

// create POSTs a new object, so the API server answers 409 if one appeared since the check
// and a foreign object is never merged into by an apply patch.
func (c *Client) create(ctx context.Context, collection string, obj map[string]any) error {
	body, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	_, err = c.doType(ctx, http.MethodPost, collection+"?fieldManager="+FieldManager, "application/json", body)
	return err
}

func isStatus(err error, code int) bool {
	var se *StatusError
	return errors.As(err, &se) && se.Code == code
}

// checkNamespace refuses an existing namespace that lacks our label unless allowed.
func (c *Client) checkNamespace(ctx context.Context, name string, allowUnmanaged bool) (found bool, err error) {
	labels, found, err := c.getLabels(ctx, "/api/v1/namespaces/"+url.PathEscape(name))
	if err != nil || !found {
		return found, err
	}
	if labels[LabelManagedBy] != ManagedByValue && !allowUnmanaged {
		return true, ErrNamespaceNotManaged
	}
	return true, nil
}

// EnsureNamespace creates the namespace with our label when it is missing and never
// patches one that already exists. An existing unlabelled one needs allowUnmanaged.
func (c *Client) EnsureNamespace(ctx context.Context, name string, allowUnmanaged bool) error {
	found, err := c.checkNamespace(ctx, name, allowUnmanaged)
	if err != nil || found {
		return err
	}
	ns := map[string]any{
		"apiVersion": "v1", "kind": "Namespace",
		"metadata": map[string]any{"name": name, "labels": map[string]any{LabelManagedBy: ManagedByValue}},
	}
	err = c.create(ctx, "/api/v1/namespaces", ns)
	if isStatus(err, http.StatusConflict) {
		// Someone created it in between: judge the real object, never our own guess.
		_, err = c.checkNamespace(ctx, name, allowUnmanaged)
	}
	return err
}

type ingressList struct {
	Items []struct {
		Metadata struct {
			Name      string            `json:"name"`
			Namespace string            `json:"namespace"`
			Labels    map[string]string `json:"labels"`
		} `json:"metadata"`
		Spec struct {
			Rules []struct {
				Host string `json:"host"`
			} `json:"rules"`
		} `json:"spec"`
	} `json:"items"`
}

// checkHost fails when an Ingress we do not own claims the host. When Ingresses cannot be
// listed (RBAC) it fails closed unless the caller allowed an unverified host.
func (c *Client) checkHost(ctx context.Context, spec AppSpec) error {
	data, err := c.Do(ctx, http.MethodGet, "/apis/networking.k8s.io/v1/ingresses", nil)
	if isStatus(err, http.StatusForbidden) || isStatus(err, http.StatusUnauthorized) {
		if spec.AllowUnverifiedHost {
			return nil
		}
		return ErrHostUnverified
	}
	if err != nil {
		return err
	}
	var list ingressList
	if err := json.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("k8sconn: decode ingresses: %w", err)
	}
	for _, it := range list.Items {
		for _, rule := range it.Spec.Rules {
			if !strings.EqualFold(rule.Host, spec.Host) {
				continue
			}
			mine := it.Metadata.Namespace == spec.Namespace && it.Metadata.Name == spec.Name &&
				ownedBy(it.Metadata.Labels, spec.Name, spec.InstanceID)
			if !mine {
				return fmt.Errorf("host %s: %w", spec.Host, ErrHostTaken)
			}
		}
	}
	return nil
}

// Apply checks the namespace, the host and every object first, so one foreign object stops
// the whole deploy before anything changes. Objects that were absent are created with POST
// (409 if one appeared meanwhile); ours are forced. Names applied so far return with an error.
func (c *Client) Apply(ctx context.Context, spec AppSpec, r Rendered) ([]string, error) {
	if _, err := c.checkNamespace(ctx, spec.Namespace, spec.AllowUnmanagedNamespace); err != nil {
		return nil, err
	}
	if r.Ingress != nil && spec.Host != "" {
		if err := c.checkHost(ctx, spec); err != nil {
			return nil, err
		}
	}
	type step struct {
		ref    ObjectRef
		obj    map[string]any
		path   string
		create bool
	}
	var steps []step
	var objs []map[string]any
	if r.Secret != nil {
		objs = append(objs, r.Secret)
	}
	objs = append(objs, r.Deployment, r.Service)
	if r.Ingress != nil {
		objs = append(objs, r.Ingress)
	}
	for i, ref := range r.Refs(spec.Namespace) {
		path := ref.Path + "/" + url.PathEscape(ref.Name)
		labels, found, err := c.getLabels(ctx, path)
		if err != nil {
			return nil, err
		}
		if found && !ownedBy(labels, spec.Name, spec.InstanceID) {
			return nil, fmt.Errorf("%s %s: %w", ref.Kind, ref.Name, ErrNotManaged)
		}
		steps = append(steps, step{ref: ref, obj: objs[i], path: path, create: !found})
	}
	if err := c.EnsureNamespace(ctx, spec.Namespace, spec.AllowUnmanagedNamespace); err != nil {
		return nil, err
	}
	names := []string{}
	for _, st := range steps {
		var err error
		if st.create {
			err = c.create(ctx, st.ref.Path, st.obj)
			if isStatus(err, http.StatusConflict) {
				err = fmt.Errorf("%w (appeared during the deploy)", ErrNotManaged)
			}
		} else {
			err = c.patch(ctx, st.path, st.obj, true)
		}
		if err != nil {
			return names, fmt.Errorf("%s %s: %w", st.ref.Kind, st.ref.Name, err)
		}
		names = append(names, st.ref.Kind+"/"+st.ref.Name)
	}
	return names, nil
}
