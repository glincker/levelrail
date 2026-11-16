package k8sconn

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Label keys let remove delete only objects Levelrail created.
const (
	LabelManagedBy = "app.kubernetes.io/managed-by"
	LabelApp       = "levelrail.dev/app"
	ManagedByValue = "levelrail"
)

var dnsLabel = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$`)

// Resources carries values the caller reads from runtime settings, so the
// renderer holds no thresholds of its own.
type Resources struct {
	CPURequest    string
	MemoryRequest string
	CPULimit      string
	MemoryLimit   string
}

// AppSpec is the subset of a Levelrail app that maps to a cluster workload.
type AppSpec struct {
	Name      string
	Namespace string
	Image     string
	Port      int
	Replicas  int
	Env       map[string]string
	// Host empty means no Ingress is rendered.
	Host         string
	IngressClass string
	// TLSIssuer, when set, adds a cert-manager cluster-issuer annotation.
	TLSIssuer string
	Resources Resources
	// RunAsUser is set only when the caller knows the image runs non-root.
	RunAsUser *int64
}

// Rendered holds the objects as maps ready for YAML or JSON.
type Rendered struct {
	Deployment map[string]any
	Service    map[string]any
	Ingress    map[string]any // nil when no host
}

// Labels returns the ownership labels for an app.
func Labels(app string) map[string]string {
	return map[string]string{LabelManagedBy: ManagedByValue, LabelApp: app}
}

func (s AppSpec) validate() error {
	switch {
	case !dnsLabel.MatchString(s.Name):
		return fmt.Errorf("render: invalid app name %q", s.Name)
	case !dnsLabel.MatchString(s.Namespace):
		return fmt.Errorf("render: invalid namespace %q", s.Namespace)
	case strings.TrimSpace(s.Image) == "":
		return fmt.Errorf("render: image is required")
	case s.Port < 1 || s.Port > 65535:
		return fmt.Errorf("render: port %d out of range", s.Port)
	case s.Replicas < 0:
		return fmt.Errorf("render: negative replicas")
	case s.Resources.CPURequest == "" || s.Resources.MemoryRequest == "" ||
		s.Resources.CPULimit == "" || s.Resources.MemoryLimit == "":
		return fmt.Errorf("render: resource requests and limits are required")
	}
	return nil
}

// Render builds the Deployment, Service and optional Ingress.
func Render(s AppSpec) (Rendered, error) {
	if err := s.validate(); err != nil {
		return Rendered{}, err
	}
	labels := Labels(s.Name)
	meta := func() map[string]any {
		return map[string]any{"name": s.Name, "namespace": s.Namespace, "labels": toAny(labels)}
	}

	sec := map[string]any{
		"privileged":               false,
		"allowPrivilegeEscalation": false,
		"capabilities":             map[string]any{"drop": []any{"ALL"}},
		"seccompProfile":           map[string]any{"type": "RuntimeDefault"},
	}
	if s.RunAsUser != nil {
		sec["runAsNonRoot"] = true
		sec["runAsUser"] = *s.RunAsUser
	}
	container := map[string]any{
		"name":            s.Name,
		"image":           s.Image,
		"ports":           []any{map[string]any{"name": "http", "containerPort": s.Port}},
		"securityContext": sec,
		"resources": map[string]any{
			"requests": map[string]any{"cpu": s.Resources.CPURequest, "memory": s.Resources.MemoryRequest},
			"limits":   map[string]any{"cpu": s.Resources.CPULimit, "memory": s.Resources.MemoryLimit},
		},
	}
	if len(s.Env) > 0 {
		keys := make([]string, 0, len(s.Env))
		for k := range s.Env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		env := make([]any, 0, len(keys))
		for _, k := range keys {
			env = append(env, map[string]any{"name": k, "value": s.Env[k]})
		}
		container["env"] = env
	}

	r := Rendered{
		Deployment: map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata":   meta(),
			"spec": map[string]any{
				"replicas": s.Replicas,
				"selector": map[string]any{"matchLabels": map[string]any{LabelApp: s.Name}},
				"template": map[string]any{
					"metadata": map[string]any{"labels": toAny(labels)},
					"spec": map[string]any{
						"automountServiceAccountToken": false,
						"hostNetwork":                  false,
						"containers":                   []any{container},
					},
				},
			},
		},
		Service: map[string]any{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata":   meta(),
			"spec": map[string]any{
				"type":     "ClusterIP",
				"selector": map[string]any{LabelApp: s.Name},
				"ports": []any{map[string]any{
					"name": "http", "port": 80, "targetPort": "http", "protocol": "TCP",
				}},
			},
		},
	}

	if s.Host != "" {
		im := meta()
		if s.TLSIssuer != "" {
			im["annotations"] = map[string]any{"cert-manager.io/cluster-issuer": s.TLSIssuer}
		}
		spec := map[string]any{
			"rules": []any{map[string]any{
				"host": s.Host,
				"http": map[string]any{"paths": []any{map[string]any{
					"path": "/", "pathType": "Prefix",
					"backend": map[string]any{"service": map[string]any{
						"name": s.Name, "port": map[string]any{"name": "http"},
					}},
				}}},
			}},
		}
		if s.IngressClass != "" {
			spec["ingressClassName"] = s.IngressClass
		}
		if s.TLSIssuer != "" {
			spec["tls"] = []any{map[string]any{
				"hosts": []any{s.Host}, "secretName": s.Name + "-tls",
			}}
		}
		r.Ingress = map[string]any{
			"apiVersion": "networking.k8s.io/v1", "kind": "Ingress",
			"metadata": im, "spec": spec,
		}
	}
	return r, nil
}

// YAML returns the objects as one multi-document stream, keys sorted.
func (r Rendered) YAML() ([]byte, error) {
	var buf bytes.Buffer
	for _, obj := range []map[string]any{r.Deployment, r.Service, r.Ingress} {
		if obj == nil {
			continue
		}
		b, err := yaml.Marshal(obj)
		if err != nil {
			return nil, err
		}
		if buf.Len() > 0 {
			buf.WriteString("---\n")
		}
		buf.Write(b)
	}
	return buf.Bytes(), nil
}

func toAny(m map[string]string) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
