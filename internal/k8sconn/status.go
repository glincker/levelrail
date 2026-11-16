package k8sconn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

// Condition is one Deployment condition, trimmed.
type Condition struct {
	Type    string
	Status  string
	Reason  string
	Message string
}

// PodState is the phase of one pod plus the reason a container is stuck, if any.
type PodState struct {
	Name   string
	Phase  string
	Reason string
}

// Status is what a deployment looks like right now.
type Status struct {
	Found           bool
	Replicas        int
	ReadyReplicas   int
	UpdatedReplicas int
	Conditions      []Condition
	Pods            []PodState
	IngressAddress  string
	// Reason is the first readable problem, empty when healthy.
	Reason string
}

var reasonText = map[string]string{
	"ImagePullBackOff":           "the image could not be pulled (wrong name, tag or a private registry)",
	"ErrImagePull":               "the image could not be pulled (wrong name, tag or a private registry)",
	"InvalidImageName":           "the image name is not valid",
	"CrashLoopBackOff":           "the container keeps crashing on start",
	"CreateContainerConfigError": "the container configuration was refused",
	"RunContainerError":          "the container could not be started",
	"ContainerCannotRun":         "the container could not be started",
	"Unschedulable":              "no node has room for the pod",
	"ProgressDeadlineExceeded":   "the rollout did not finish in time",
}

func readable(reason string) string {
	if t, ok := reasonText[reason]; ok {
		return t
	}
	return reason
}

type deploymentDoc struct {
	Metadata struct {
		Labels map[string]string `json:"labels"`
	} `json:"metadata"`
	Status struct {
		Replicas        int `json:"replicas"`
		ReadyReplicas   int `json:"readyReplicas"`
		UpdatedReplicas int `json:"updatedReplicas"`
		Conditions      []struct {
			Type, Status, Reason, Message string
		} `json:"conditions"`
	} `json:"status"`
}

// DeploymentStatus reads the deployment, its pods and its ingress. It only reads
// objects that carry our labels, so a foreign object of the same name looks missing.
func (c *Client) DeploymentStatus(ctx context.Context, namespace, app string) (Status, error) {
	ns := url.PathEscape(namespace)
	data, err := c.Do(ctx, http.MethodGet, "/apis/apps/v1/namespaces/"+ns+"/deployments/"+url.PathEscape(app), nil)
	var se *StatusError
	if errors.As(err, &se) && se.Code == http.StatusNotFound {
		return Status{}, nil
	}
	if err != nil {
		return Status{}, err
	}
	var d deploymentDoc
	if err := json.Unmarshal(data, &d); err != nil {
		return Status{}, fmt.Errorf("k8sconn: decode deployment: %w", err)
	}
	if !ownedBy(d.Metadata.Labels, app) {
		return Status{}, nil
	}
	st := Status{Found: true, Replicas: d.Status.Replicas, ReadyReplicas: d.Status.ReadyReplicas, UpdatedReplicas: d.Status.UpdatedReplicas}
	for _, cd := range d.Status.Conditions {
		st.Conditions = append(st.Conditions, Condition{Type: cd.Type, Status: cd.Status, Reason: cd.Reason, Message: cd.Message})
		if cd.Type == "Progressing" && cd.Status == "False" && st.Reason == "" {
			st.Reason = readable(cd.Reason)
		}
	}
	sel := url.QueryEscape(LabelApp + "=" + app)
	if pdata, err := c.Do(ctx, http.MethodGet, "/api/v1/namespaces/"+ns+"/pods?labelSelector="+sel, nil); err == nil {
		st.Pods = podStates(pdata)
		for _, p := range st.Pods {
			if p.Reason != "" {
				st.Reason = readable(p.Reason)
				break
			}
		}
	}
	if idata, err := c.Do(ctx, http.MethodGet, "/apis/networking.k8s.io/v1/namespaces/"+ns+"/ingresses/"+url.PathEscape(app), nil); err == nil {
		st.IngressAddress = ingressAddress(idata)
	}
	return st, nil
}

func podStates(data []byte) []PodState {
	var l struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Status struct {
				Phase      string `json:"phase"`
				Conditions []struct {
					Type, Status, Reason string
				} `json:"conditions"`
				ContainerStatuses []struct {
					State struct {
						Waiting *struct {
							Reason string `json:"reason"`
						} `json:"waiting"`
					} `json:"state"`
				} `json:"containerStatuses"`
			} `json:"status"`
		} `json:"items"`
	}
	if json.Unmarshal(data, &l) != nil {
		return nil
	}
	out := make([]PodState, 0, len(l.Items))
	for _, it := range l.Items {
		p := PodState{Name: it.Metadata.Name, Phase: it.Status.Phase}
		for _, cs := range it.Status.ContainerStatuses {
			if w := cs.State.Waiting; w != nil && w.Reason != "" && w.Reason != "ContainerCreating" && p.Reason == "" {
				p.Reason = w.Reason
			}
		}
		for _, cd := range it.Status.Conditions {
			if cd.Type == "PodScheduled" && cd.Status == "False" && p.Reason == "" {
				p.Reason = cd.Reason
			}
		}
		out = append(out, p)
	}
	return out
}

func ingressAddress(data []byte) string {
	var i struct {
		Status struct {
			LoadBalancer struct {
				Ingress []struct{ IP, Hostname string } `json:"ingress"`
			} `json:"loadBalancer"`
		} `json:"status"`
	}
	if json.Unmarshal(data, &i) != nil || len(i.Status.LoadBalancer.Ingress) == 0 {
		return ""
	}
	a := i.Status.LoadBalancer.Ingress[0]
	if a.IP != "" {
		return a.IP
	}
	return a.Hostname
}
