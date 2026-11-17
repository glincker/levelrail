package k8sconn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

// ErrLogTarget means the deployment, pod or container is not one of ours.
var ErrLogTarget = errors.New("k8sconn: no such pod or container for this app")

// LogResult is one bounded log read.
type LogResult struct {
	Pod       string
	Container string
	Text      string
	Truncated bool
}

type podDoc struct {
	Metadata struct {
		Name   string            `json:"name"`
		Labels map[string]string `json:"labels"`
	} `json:"metadata"`
	Spec struct {
		Containers []struct {
			Name string `json:"name"`
		} `json:"containers"`
	} `json:"spec"`
	Status struct {
		Phase string `json:"phase"`
	} `json:"status"`
}

// PodLogs reads the tail of one pod of an app we own. The pod must come from our own label
// selector, so an arbitrary pod name cannot be read. maxBytes bounds the text returned.
func (c *Client) PodLogs(ctx context.Context, namespace, app, instance, pod, container string, tail int, maxBytes int64) (LogResult, error) {
	st, err := c.DeploymentStatus(ctx, namespace, app, instance)
	if err != nil {
		return LogResult{}, err
	}
	if !st.Found {
		return LogResult{}, ErrLogTarget
	}
	ns := url.PathEscape(namespace)
	sel := LabelApp + "=" + app
	if instance != "" {
		sel += "," + LabelInstance + "=" + instance
	}
	data, err := c.Do(ctx, http.MethodGet, "/api/v1/namespaces/"+ns+"/pods?labelSelector="+url.QueryEscape(sel), nil)
	if err != nil {
		return LogResult{}, err
	}
	var l struct {
		Items []podDoc `json:"items"`
	}
	if err := json.Unmarshal(data, &l); err != nil {
		return LogResult{}, fmt.Errorf("k8sconn: decode pods: %w", err)
	}
	chosen, ok := pickPod(l.Items, pod)
	if !ok {
		return LogResult{}, ErrLogTarget
	}
	ctr := container
	if ctr == "" && len(chosen.Spec.Containers) > 0 {
		ctr = chosen.Spec.Containers[0].Name
	}
	known := false
	for _, k := range chosen.Spec.Containers {
		known = known || k.Name == ctr
	}
	if !known {
		return LogResult{}, ErrLogTarget
	}
	q := url.Values{"container": {ctr}, "tailLines": {strconv.Itoa(tail)}, "limitBytes": {strconv.FormatInt(maxBytes+1, 10)}}
	text, err := c.getText(ctx, "/api/v1/namespaces/"+ns+"/pods/"+url.PathEscape(chosen.Metadata.Name)+"/log?"+q.Encode(), maxBytes)
	if err != nil {
		return LogResult{}, err
	}
	out := LogResult{Pod: chosen.Metadata.Name, Container: ctr, Text: text}
	if int64(len(text)) > maxBytes {
		out.Text, out.Truncated = truncateBytes(text, int(maxBytes)), true
	}
	return out, nil
}

// pickPod returns the named pod, or the first running one, or the first one.
func pickPod(items []podDoc, name string) (podDoc, bool) {
	for _, p := range items {
		if name != "" && p.Metadata.Name == name {
			return p, true
		}
	}
	if name != "" || len(items) == 0 {
		return podDoc{}, false
	}
	for _, p := range items {
		if p.Status.Phase == "Running" {
			return p, true
		}
	}
	return items[0], true
}

// truncateBytes cuts on a rune boundary so a multi-byte character is never split.
func truncateBytes(s string, n int) string {
	if n >= len(s) {
		return s
	}
	for n > 0 && !isRuneStart(s[n]) {
		n--
	}
	return s[:n]
}

func isRuneStart(b byte) bool { return b&0xC0 != 0x80 }

// getText reads a plain text body. The log endpoint answers text, so Accept is open, and
// the read stops a little past maxBytes instead of failing on a long log.
func (c *Client) getText(ctx context.Context, path string, maxBytes int64) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "text/plain, */*")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", classify(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", &StatusError{Code: resp.StatusCode}
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+readSlack))
	if err != nil {
		return "", classify(err)
	}
	return string(b), nil
}

// readSlack lets the read see one byte past the cap so truncation is detectable.
const readSlack = 8
