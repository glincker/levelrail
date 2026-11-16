package k8sconn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"unicode/utf8"
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
	// Message is the last terminated or waiting message, truncated; never env values.
	Message string
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
	Spec struct {
		Template struct {
			Spec struct {
				ImagePullSecrets []struct {
					Name string `json:"name"`
				} `json:"imagePullSecrets"`
			} `json:"spec"`
		} `json:"template"`
	} `json:"spec"`
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
// objects that carry our labels and this install's instance id, so a foreign object of the
// same name looks missing. An empty instance skips the instance check.
func (c *Client) DeploymentStatus(ctx context.Context, namespace, app, instance string) (Status, error) {
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
	if !ownedBy(d.Metadata.Labels, app, instance) {
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
				if len(d.Spec.Template.Spec.ImagePullSecrets) > 0 && (p.Reason == "ErrImagePull" || p.Reason == "ImagePullBackOff") {
					st.Reason = fmt.Sprintf("the image could not be pulled with pull secret %s (check the registry, username and password or token)", d.Spec.Template.Spec.ImagePullSecrets[0].Name)
				}
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
					State     containerState `json:"state"`
					LastState containerState `json:"lastState"`
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
			if p.Reason != "" {
				break
			}
			switch {
			case cs.State.Waiting != nil && cs.State.Waiting.Reason != "" && cs.State.Waiting.Reason != "ContainerCreating":
				p.Reason = cs.State.Waiting.Reason
				p.Message = cs.State.Waiting.Message
				// A crash loop keeps the last exit message in lastState.
				if t := cs.LastState.Terminated; t != nil && t.Message != "" {
					p.Message = t.Message
				}
			case cs.State.Terminated != nil && cs.State.Terminated.ExitCode != 0:
				p.Reason = nonEmpty(cs.State.Terminated.Reason, "Error")
				p.Message = cs.State.Terminated.Message
			case cs.LastState.Terminated != nil && cs.LastState.Terminated.ExitCode != 0:
				p.Reason = nonEmpty(cs.LastState.Terminated.Reason, "Error")
				p.Message = cs.LastState.Terminated.Message
			}
		}
		p.Message = truncateMessage(p.Message)
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

type containerState struct {
	Waiting *struct {
		Reason  string `json:"reason"`
		Message string `json:"message"`
	} `json:"waiting"`
	Terminated *struct {
		Reason   string `json:"reason"`
		Message  string `json:"message"`
		ExitCode int    `json:"exitCode"`
	} `json:"terminated"`
}

func nonEmpty(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

// messageMax keeps cluster-authored text small in a status row.
const messageMax = 300

// truncateMessage cuts on a rune boundary so a multi-byte character is never split.
func truncateMessage(s string) string {
	if len(s) <= messageMax {
		return s
	}
	cut := messageMax
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}
