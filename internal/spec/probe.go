package spec

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/probe"
	"gopkg.in/yaml.v3"
)

// StatusCodes is health.*.expected_status: "200-399", 204, or a list
// mixing codes and ranges. It is kept in its normalized string form.
type StatusCodes string

// UnmarshalYAML accepts a scalar or a sequence of scalars.
func (s *StatusCodes) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		*s = StatusCodes(node.Value)
		return nil
	case yaml.SequenceNode:
		parts := make([]string, 0, len(node.Content))
		for _, item := range node.Content {
			if item.Kind != yaml.ScalarNode {
				return fmt.Errorf("expected_status: list entries must be status codes or ranges")
			}
			parts = append(parts, item.Value)
		}
		*s = StatusCodes(strings.Join(parts, ","))
		return nil
	default:
		return fmt.Errorf("expected_status: must be a status code, a range like 200-399, or a list of them")
	}
}

// UnmarshalJSON accepts a string, a number, or an array of either.
func (s *StatusCodes) UnmarshalJSON(data []byte) error {
	var list []json.RawMessage
	if err := json.Unmarshal(data, &list); err == nil {
		parts := make([]string, 0, len(list))
		for _, raw := range list {
			part, err := statusScalar(raw)
			if err != nil {
				return err
			}
			parts = append(parts, part)
		}
		*s = StatusCodes(strings.Join(parts, ","))
		return nil
	}
	part, err := statusScalar(data)
	if err != nil {
		return err
	}
	*s = StatusCodes(part)
	return nil
}

func statusScalar(raw json.RawMessage) (string, error) {
	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		return str, nil
	}
	var n int
	if err := json.Unmarshal(raw, &n); err == nil {
		return strconv.Itoa(n), nil
	}
	return "", fmt.Errorf("expected_status: %s is not a status code or range", string(raw))
}

// ExecCommand is health.*.exec: a list is run as argv, a string through /bin/sh -c.
type ExecCommand []string

// UnmarshalYAML accepts a shell string or an argv list.
func (e *ExecCommand) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		*e = probe.ShellCommand(node.Value)
		return nil
	case yaml.SequenceNode:
		var argv []string
		if err := node.Decode(&argv); err != nil {
			return fmt.Errorf("exec: %w", err)
		}
		*e = argv
		return nil
	default:
		return fmt.Errorf("exec: must be a command string or a list of arguments")
	}
}

// UnmarshalJSON accepts a shell string or an argv array.
func (e *ExecCommand) UnmarshalJSON(data []byte) error {
	var script string
	if err := json.Unmarshal(data, &script); err == nil {
		*e = probe.ShellCommand(script)
		return nil
	}
	var argv []string
	if err := json.Unmarshal(data, &argv); err != nil {
		return fmt.Errorf("exec: must be a command string or a list of arguments")
	}
	*e = argv
	return nil
}

// ProbeConfig converts p into internal/probe's shape.
func (p Probe) ProbeConfig() (probe.Config, error) {
	interval, err := parseProbeDuration("interval", p.Interval)
	if err != nil {
		return probe.Config{}, err
	}
	timeout, err := parseProbeDuration("timeout", p.Timeout)
	if err != nil {
		return probe.Config{}, err
	}
	return probe.Config{
		Path:            p.Path,
		Scheme:          p.Scheme,
		Host:            p.Host,
		TLSSkipVerify:   p.TLSSkipVerify,
		FollowRedirects: p.FollowRedirects,
		ExpectedStatus:  string(p.ExpectedStatus),
		Exec:            []string(p.Exec),
		Interval:        interval,
		Timeout:         timeout,
	}, nil
}

func parseProbeDuration(field, s string) (time.Duration, error) {
	if s == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid duration %q", field, s)
	}
	return d, nil
}

// Validate checks both probes, naming service in any error.
func (h *Health) Validate(service string) error {
	if h == nil {
		return nil
	}
	for _, pr := range []struct {
		name  string
		probe *Probe
	}{{"readiness", h.Readiness}, {"liveness", h.Liveness}} {
		if pr.probe == nil {
			continue
		}
		cfg, err := pr.probe.ProbeConfig()
		if err == nil {
			err = cfg.Validate()
		}
		if err != nil {
			return fmt.Errorf("spec: service %q: health.%s: %w", service, pr.name, err)
		}
	}
	return nil
}
