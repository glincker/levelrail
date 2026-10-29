package iac

import (
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// connectionFields are the database values an env var may reference; the
// control plane resolves them at container create, so no value is ever read here.
var connectionFields = map[string]bool{"url": true, "direct_url": true, "host": true, "port": true, "username": true, "password": true, "database": true}

const defaultConnectionField = "url"

// parseFromRef reads { from: { database, field } } or the app.yaml string
// form "<database>.<field>" into the canonical "database.field".
func parseFromRef(n *yaml.Node, path string, s *issueSink) (string, bool) {
	var db, field string
	switch n.Kind {
	case yaml.MappingNode:
		var ref struct {
			Database string `yaml:"database"`
			Field    string `yaml:"field"`
		}
		if err := n.Decode(&ref); err != nil {
			s.at(path, "%v", err)
			return "", false
		}
		db, field = ref.Database, ref.Field
	case yaml.ScalarNode:
		parts := strings.Split(n.Value, ".")
		if len(parts) < 2 {
			s.at(path, "from %q must be <database>.<field> or { database, field }", n.Value)
			return "", false
		}
		db, field = parts[len(parts)-2], parts[len(parts)-1]
	}
	if field == "" {
		field = defaultConnectionField
	}
	if !appNameRe.MatchString(db) {
		s.at(path, "database %q is not a valid database name", db)
		return "", false
	}
	if !connectionFields[field] {
		s.at(path, "field %q is not one of url, host, port, username, password, database", field)
		return "", false
	}
	return db + "." + field, true
}

func splitConnRef(ref string) (db, field string) {
	db, field, _ = strings.Cut(ref, ".")
	return db, field
}

// jobFields is the comparable shape of one scheduled job on both sides.
type jobFields struct {
	Schedule        string   `json:"schedule"`
	Command         []string `json:"command,omitempty"`
	Timezone        string   `json:"timezone,omitempty"`
	Mode            string   `json:"mode"`
	Concurrency     string   `json:"concurrency"`
	Enabled         bool     `json:"enabled"`
	Timeout         string   `json:"timeout,omitempty"`
	Retries         int      `json:"retries,omitempty"`
	RetryBackoff    string   `json:"retryBackoff,omitempty"`
	CatchUp         string   `json:"catchUp,omitempty"`
	CatchUpLookback string   `json:"catchUpLookback,omitempty"`
	CPU             float64  `json:"cpu,omitempty"`
	Memory          string   `json:"memory,omitempty"`
	WarnAfter       string   `json:"warnAfter,omitempty"`
}

// wireJob is the jobs API resource, limited to what apply manages.
type wireJob struct {
	ID                     string   `json:"id,omitempty"`
	Name                   string   `json:"name"`
	Mode                   string   `json:"mode"`
	Command                []string `json:"command"`
	Schedule               string   `json:"schedule"`
	Timezone               string   `json:"timezone,omitempty"`
	Enabled                bool     `json:"enabled"`
	ConcurrencyPolicy      string   `json:"concurrency_policy"`
	TimeoutSeconds         int      `json:"timeout_seconds"`
	MaxRetries             int      `json:"max_retries"`
	RetryBackoffSeconds    int      `json:"retry_backoff_seconds"`
	CatchUp                string   `json:"catch_up,omitempty"`
	CatchUpLookbackSeconds int      `json:"catch_up_lookback_seconds"`
	CPULimitMillis         int      `json:"cpu_limit_millis"`
	MemoryLimitMB          int      `json:"memory_limit_mb"`
	WarnAfterSeconds       int      `json:"warn_after_seconds"`
	Source                 string   `json:"source,omitempty"`
}

const (
	jobSourceSpec  = store.ScheduledTaskSourceSpec
	jobCatchUpSkip = store.ScheduledTaskCatchUpSkip
)

func secondsString(n int) string { return fmtDuration(time.Duration(n) * time.Second) }

func durationSeconds(s string) int {
	d, _ := time.ParseDuration(s)
	return int(d / time.Second)
}

// jobsFromSpec mirrors the deploy pipeline's app.yaml job defaults, so the
// same job declared in either place behaves the same.
func jobsFromSpec(jobs []spec.Job, s *issueSink) map[string]jobFields {
	if len(jobs) == 0 {
		return nil
	}
	out := make(map[string]jobFields, len(jobs))
	for i, j := range jobs {
		path := "spec.service.jobs." + j.Name
		if j.Name == "" {
			s.at("spec.service.jobs", "job %d needs a name: apply matches jobs by name", i+1)
			continue
		}
		f := jobFields{Schedule: j.Schedule, Command: j.Command, Timezone: j.Timezone, Mode: store.ScheduledTaskModeFresh, Concurrency: store.ScheduledTaskConcurrencyForbid,
			Enabled: j.Enabled == nil || *j.Enabled, Retries: j.Retries, CatchUp: jobCatchUpSkip, CPU: j.CPU}
		if f.Timezone == "UTC" {
			f.Timezone = ""
		}
		if j.Mode == spec.JobModeExec {
			f.Mode = store.ScheduledTaskModeExec
		}
		if j.Concurrency != "" {
			f.Concurrency = j.Concurrency
		}
		if j.CatchUp == spec.JobCatchUpRunOnce {
			f.CatchUp = store.ScheduledTaskCatchUpRunOnce
		}
		for dst, raw := range map[*string]string{&f.Timeout: j.Timeout, &f.RetryBackoff: j.RetryBackoff, &f.CatchUpLookback: j.CatchUpLookback, &f.WarnAfter: j.WarnAfter} {
			v, err := normDuration(raw)
			if err != nil {
				s.at(path, "%v", err)
			}
			*dst = v
		}
		if j.Memory != "" {
			b, err := parseMemory(strings.TrimSpace(j.Memory))
			if err != nil {
				s.at(path+".memory", "%v", err)
			}
			f.Memory = memoryString(b)
		}
		out[j.Name] = f
	}
	return out
}

func jobFromWire(w wireJob) jobFields {
	f := jobFields{Schedule: w.Schedule, Command: w.Command, Timezone: w.Timezone, Mode: w.Mode, Concurrency: w.ConcurrencyPolicy,
		Enabled: w.Enabled, Timeout: secondsString(w.TimeoutSeconds), Retries: w.MaxRetries, RetryBackoff: secondsString(w.RetryBackoffSeconds),
		CatchUp: w.CatchUp, CatchUpLookback: secondsString(w.CatchUpLookbackSeconds), CPU: float64(w.CPULimitMillis) / 1000, WarnAfter: secondsString(w.WarnAfterSeconds)}
	if f.Timezone == "UTC" {
		f.Timezone = ""
	}
	if f.CatchUp == "" {
		f.CatchUp = jobCatchUpSkip
	}
	if w.MemoryLimitMB > 0 {
		f.Memory = memoryString(int64(w.MemoryLimitMB) << 20)
	}
	return f
}

func jobBody(name string, f jobFields) map[string]any {
	tz := f.Timezone
	if tz == "" {
		tz = "UTC"
	}
	b := map[string]any{"name": name, "mode": f.Mode, "command": f.Command, "schedule": f.Schedule, "timezone": tz, "enabled": f.Enabled,
		"concurrency_policy": f.Concurrency, "timeout_seconds": durationSeconds(f.Timeout), "max_retries": f.Retries,
		"retry_backoff_seconds": durationSeconds(f.RetryBackoff), "catch_up": f.CatchUp, "catch_up_lookback_seconds": durationSeconds(f.CatchUpLookback),
		"cpu_limit_millis": int(f.CPU * 1000), "warn_after_seconds": durationSeconds(f.WarnAfter), "memory_limit_mb": 0}
	if f.Memory != "" {
		m, _ := parseMemory(f.Memory)
		b["memory_limit_mb"] = int(m >> 20)
	}
	return b
}
