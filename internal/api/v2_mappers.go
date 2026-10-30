package api

import (
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Kind names of the v2 uniform resource view, keyed by their route segment.
const (
	v2KindApp      = "App"
	v2KindNode     = "Node"
	v2KindDatabase = "Database"

	v2PathApps      = "apps"
	v2PathNodes     = "nodes"
	v2PathDatabases = "databases"

	v2TagsAnnotation = "/tags"
)

// v2Object is one resource in the uniform view: v1 data reshaped, never stored twice.
type v2Object struct {
	APIVersion string     `json:"apiVersion"`
	Kind       string     `json:"kind"`
	Metadata   v2Metadata `json:"metadata"`
	Spec       any        `json:"spec,omitempty"`
	Status     any        `json:"status,omitempty"`
}

type v2Metadata struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
	ResourceVersion   string            `json:"resourceVersion,omitempty"`
	CreationTimestamp string            `json:"creationTimestamp,omitempty"`
}

type v2ListMeta struct {
	ResourceVersion string `json:"resourceVersion"`
	Continue        string `json:"continue,omitempty"`
}

type v2List struct {
	APIVersion string     `json:"apiVersion"`
	Kind       string     `json:"kind"`
	Metadata   v2ListMeta `json:"metadata"`
	Items      []v2Object `json:"items"`
}

// v2APIVersion derives the group from the brand domain so no product name is hardcoded.
func v2APIVersion(domain string) string {
	host, _, _ := strings.Cut(domain, "/")
	if host == "" {
		return "v2"
	}
	return host + "/v2"
}

func v2Version(v int64) string {
	if v <= 0 {
		return ""
	}
	return strconv.FormatInt(v, 10)
}

// v2AppObject maps an already redacted v1 app body; tags become an annotation.
func v2AppObject(apiVersion string, res appResource, svc store.DesiredService, tags []string, status *appStatusSummary, rv int64) v2Object {
	meta := v2Metadata{
		Name:            svc.Name,
		Namespace:       svc.ProjectID,
		Labels:          svc.Labels,
		ResourceVersion: v2Version(rv),
	}
	if len(tags) > 0 {
		meta.Annotations = map[string]string{strings.TrimSuffix(apiVersion, "/v2") + v2TagsAnnotation: strings.Join(tags, ",")}
	}
	obj := v2Object{APIVersion: apiVersion, Kind: v2KindApp, Metadata: meta, Spec: res}
	if status != nil {
		obj.Status = status
	}
	return obj
}

func v2NodeObject(apiVersion string, res nodeResource, rv int64) v2Object {
	labels := map[string]string{}
	for k, v := range res.Labels {
		labels[k] = v
	}
	for k, v := range map[string]string{"region": res.Region, "zone": res.Zone, "provider": res.Provider} {
		if v != "" {
			if _, set := labels[k]; !set {
				labels[k] = v
			}
		}
	}
	if len(labels) == 0 {
		labels = nil
	}
	meta := v2Metadata{Name: res.Name, Labels: labels, ResourceVersion: v2Version(rv)}
	if !res.CreatedAt.IsZero() {
		meta.CreationTimestamp = res.CreatedAt.UTC().Format(time.RFC3339)
	}
	return v2Object{
		APIVersion: apiVersion,
		Kind:       v2KindNode,
		Metadata:   meta,
		Spec:       res,
		Status:     map[string]string{"phase": res.Status, "reason": res.StatusReason},
	}
}

func v2DatabaseObject(apiVersion string, res databaseResource, rv int64) v2Object {
	return v2Object{
		APIVersion: apiVersion,
		Kind:       v2KindDatabase,
		Metadata:   v2Metadata{Name: res.Name, Namespace: res.ProjectID, ResourceVersion: v2Version(rv)},
		Spec:       res,
	}
}
