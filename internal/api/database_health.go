package api

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/glincker/levelrail/internal/dbprovision"
	"github.com/glincker/levelrail/internal/reconcile"
	"github.com/glincker/levelrail/internal/secretmask"
)

const (
	crashStatusLabel     = "Crash looping"
	notRunningStatusText = "Not running"
	crashMessageMax      = 240
	crashLogMarker       = "Last output: "
)

// incompatibleLogMarkers are the engine log phrases that mean the data directory belongs to another major version.
var incompatibleLogMarkers = []string{"incompatible with server", "database files are incompatible", "was initialized by postgresql version", "initialized by postgresql version"}

var onDiskVersionPattern = regexp.MustCompile(`(?i)initialized by (?:postgresql|mysql|mariadb) version ([0-9]+(?:\.[0-9]+)?)`)

// crashLoopHint turns a crash loop message into the owner's next step. The detected on-disk version is named when the log carries it.
func crashLoopHint(message string) string {
	low := strings.ToLower(message)
	for _, m := range incompatibleLogMarkers {
		if strings.Contains(low, m) {
			if mm := onDiskVersionPattern.FindStringSubmatch(message); mm != nil {
				return fmt.Sprintf("The data directory was written by another major version (on disk: %s). Change the version in Settings to match it, or restore a backup into a new database.", mm[1])
			}
			return "The data directory was written by another major version. Change the version in Settings to match the data, or restore a backup into a new database."
		}
	}
	return "The container keeps exiting. Open the database logs to see why."
}

// crashLoopLastLine returns the masked, truncated last log line carried by a crash loop condition message.
func crashLoopLastLine(message string) string {
	line := message
	if i := strings.LastIndex(message, crashLogMarker); i >= 0 {
		line = message[i+len(crashLogMarker):]
	}
	line = strings.TrimSpace(secretmask.Mask(line))
	if len(line) > crashMessageMax {
		line = line[:crashMessageMax] + "..."
	}
	return line
}

// summarizeDatabaseConditions is summarizeAppConditions plus the honest crash states a database reports.
func summarizeDatabaseConditions(conditions []reconcile.Condition) appStatusSummary {
	s := summarizeAppConditions(conditions)
	if s.Variant != "destructive" {
		return s
	}
	switch s.Reason {
	case dbprovision.ReasonCrashLoop:
		s.Label = crashStatusLabel
		s.Hint = crashLoopHint(s.Message)
		s.Message = crashLoopLastLine(s.Message)
	case "VanishedAfterStart":
		s.Label = notRunningStatusText
		s.Hint = "The container stopped right after it started. Open the database logs to see why."
		s.Message = crashLoopLastLine(s.Message)
	}
	return s
}

type crashLoopingDatabase struct {
	Name    string
	Message string
	Hint    string
}

// crashLoopingDatabases lists databases whose Ready condition reports a crash loop.
func (rt *Router) crashLoopingDatabases(ctx context.Context) ([]crashLoopingDatabase, error) {
	dbs, err := rt.databases.ListDesiredDatabases(ctx)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(dbs))
	for i, d := range dbs {
		names[i] = databaseControllerName(d.Name)
	}
	conds, err := rt.deploys.GetConditionsForControllers(ctx, names)
	if err != nil {
		return nil, err
	}
	var out []crashLoopingDatabase
	for _, d := range dbs {
		s := summarizeDatabaseConditions(conds[databaseControllerName(d.Name)])
		if s.Label == crashStatusLabel {
			out = append(out, crashLoopingDatabase{Name: d.Name, Message: s.Message, Hint: s.Hint})
		}
	}
	return out, nil
}

func (rt *Router) doctorCheckDatabaseCrashLoop(ctx context.Context) doctorCheckResource {
	const code, name = "database_crash_loop", "Crash looping databases"
	if rt.databases == nil || rt.deploys == nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "Database status is not available."}
	}
	loops, err := rt.crashLoopingDatabases(ctx)
	if err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "Database status could not be read."}
	}
	return databaseCrashLoopResult(code, name, loops)
}

func databaseCrashLoopResult(code, name string, loops []crashLoopingDatabase) doctorCheckResource {
	if len(loops) == 0 {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: "No database is crash looping."}
	}
	parts := make([]string, 0, len(loops))
	for _, l := range loops {
		parts = append(parts, fmt.Sprintf("%s (%s)", l.Name, l.Message))
	}
	return doctorCheckResource{
		Code: code, Name: name, Status: doctorStatusFail,
		Message: "Crash looping: " + strings.Join(parts, "; "),
		Fix:     loops[0].Hint,
	}
}
