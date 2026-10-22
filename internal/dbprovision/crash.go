package dbprovision

import (
	"encoding/json"
	"regexp"
	"strings"
)

// StepLogTail is the pseudo step that carries a crashed container's output; it is not a provisioning step.
const StepLogTail = "log_tail"

// StatusInfo marks an event that records data rather than progress.
const StatusInfo = "info"

// Typed crash reasons.
const (
	CausePermission  = "permission"
	CauseOutOfMemory = "out_of_memory"
	CausePortInUse   = "port_in_use"
	CauseBadVolume   = "bad_volume"
	CauseBadConfig   = "bad_config"
)

// Fix ids for crash causes. They are guided: the client shows the hint, none is applied server side.
const (
	FixGrantCapabilities = "grant_capabilities"
	FixRaiseMemory       = "raise_memory"
	FixFreePort          = "free_port"
	FixResetVolume       = "reset_volume"
	FixReviewConfig      = "review_config"
)

// ReasonCrashLoop is the Ready-condition reason when the container restarts repeatedly while starting.
const ReasonCrashLoop = "CrashLoop"

// Cause is a classified reason a database container stopped, with a typed fix.
type Cause struct {
	Kind string `json:"kind"`
	Hint string `json:"hint"`
	Fix  *Fix   `json:"fix,omitempty"`
}

// CrashInfo is the recorded output and exit facts of a crashed container.
type CrashInfo struct {
	Tail      string `json:"tail"`
	ExitCode  int    `json:"exit_code"`
	OOMKilled bool   `json:"oom_killed,omitempty"`
	Restarts  int    `json:"restarts,omitempty"`
}

const (
	maxTailLines = 30
	exitSigKill  = 137
)

type causeRule struct {
	kind  string
	hint  string
	fix   Fix
	match []string
}

// Order matters: the first rule with a matching phrase wins.
var causeRules = []causeRule{
	{CausePortInUse, "Another process already uses the port. Free it or pick another port.",
		Fix{ID: FixFreePort, Label: "Free the port or choose another"},
		[]string{"address already in use", "port is already allocated", "bind: address", "eaddrinuse"}},
	{CauseOutOfMemory, "The database ran out of memory. Raise its memory limit.",
		Fix{ID: FixRaiseMemory, Label: "Raise the memory limit"},
		[]string{"out of memory", "oom-kill", "cannot allocate memory", "can't allocate memory"}},
	{CauseBadVolume, "The data volume is corrupt or was written by an incompatible version. Restore a backup or start from a fresh volume.",
		Fix{ID: FixResetVolume, Label: "Restore a backup or reset the volume", Confirm: true},
		[]string{"incompatible with server", "was initialized by postgresql version", "invalid checkpoint", "database files are incompatible",
			"bad file format reading the append only file", "can't handle rdb format", "fatal error loading the db", "corrupt", "bad magic number", "invalid magic number"}},
	{CauseBadConfig, "The database rejected its configuration. Review the settings you changed.",
		Fix{ID: FixReviewConfig, Label: "Review the configuration"},
		[]string{"fatal config file error", "unrecognized configuration parameter", "invalid value for parameter", "unknown configuration", "invalid configuration", "unknown option", "bad directive", "error in config", "unknown variable"}},
	{CausePermission, "The container was denied a permission or capability. Check the host security profile and volume ownership.",
		Fix{ID: FixGrantCapabilities, Label: "Allow the capability or fix volume ownership"},
		[]string{"operation not permitted", "permission denied", "setpriv", "bounding set", "wrong ownership", "eperm"}},
}

// Classify maps a crashed container's output and exit facts to a typed cause, or nil when none matches.
func Classify(tail string, exitCode int, oomKilled bool) *Cause {
	if oomKilled || exitCode == exitSigKill {
		return causeFor(CauseOutOfMemory)
	}
	low := strings.ToLower(tail)
	for _, r := range causeRules {
		for _, m := range r.match {
			if strings.Contains(low, m) {
				return causeFor(r.kind)
			}
		}
	}
	return nil
}

func causeFor(kind string) *Cause {
	for _, r := range causeRules {
		if r.kind == kind {
			f := r.fix
			return &Cause{Kind: r.kind, Hint: r.hint, Fix: &f}
		}
	}
	return nil
}

var (
	secretKV  = regexp.MustCompile(`(?i)((?:pass(?:word|wd)?|pwd|secret|token|api[_-]?key|access[_-]?key|auth(?:orization)?)[\w-]*\s*[=:]\s*)("[^"]*"|'[^']*'|\S+)`)
	urlCreds  = regexp.MustCompile(`(?i)(://[^/\s:@]+:)[^@\s/]+@`)
	bearerTok = regexp.MustCompile(`(?i)(bearer\s+)\S+`)
)

// RedactTail hides anything that looks like a password or token and keeps the last maxTailLines lines.
func RedactTail(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > maxTailLines {
		lines = lines[len(lines)-maxTailLines:]
	}
	out := strings.Join(lines, "\n")
	out = urlCreds.ReplaceAllString(out, "${1}***@")
	out = bearerTok.ReplaceAllString(out, "${1}***")
	return secretKV.ReplaceAllString(out, "${1}***")
}

// EncodeCrash serializes info for a log_tail event, redacting the tail.
func EncodeCrash(info CrashInfo) string {
	info.Tail = RedactTail(info.Tail)
	b, err := json.Marshal(info)
	if err != nil {
		return ""
	}
	return string(b)
}

func decodeCrash(detail string) (CrashInfo, bool) {
	var info CrashInfo
	if json.Unmarshal([]byte(detail), &info) != nil {
		return info, false
	}
	return info, true
}

// LogTail records a crashed container's output on the current attempt. A nil Tracker ignores it.
func (t *Tracker) LogTail(info CrashInfo) {
	if payload := EncodeCrash(info); payload != "" {
		t.emit(StepLogTail, StatusInfo, payload, 0, 0)
	}
}
