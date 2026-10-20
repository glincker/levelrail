package spec

import (
	"strings"
	"testing"
)

func TestParse_Jobs(t *testing.T) {
	base := "version: 1\nservices:\n  web:\n    build:\n      type: image\n      image: nginx:1\n    port: 80\n"
	tests := []struct {
		name    string
		jobs    string
		wantErr string
	}{
		{
			name: "valid full job",
			jobs: `    jobs:
      - name: nightly-migrate
        schedule: "0 3 * * *"
        command: ["sh", "-c", "bin/migrate"]
        timezone: Europe/Berlin
        mode: fresh
        concurrency: forbid
        timeout: 10m
        retries: 2
        retryBackoff: 30s
        catchUp: run-once
        catchUpLookback: 1h
        cpu: 0.5
        memory: 256Mi
        warnAfter: 20m
        enabled: false
`,
		},
		{
			name:    "bad cron",
			jobs:    "    jobs:\n      - {name: a, schedule: \"61 * * * *\", command: [x]}\n",
			wantErr: "schedule",
		},
		{
			name:    "bad timezone",
			jobs:    "    jobs:\n      - {name: a, schedule: \"* * * * *\", command: [x], timezone: Mars/Base}\n",
			wantErr: "timezone",
		},
		{
			name:    "duplicate name",
			jobs:    "    jobs:\n      - {name: a, schedule: \"* * * * *\", command: [x]}\n      - {name: a, schedule: \"* * * * *\", command: [y]}\n",
			wantErr: "duplicate",
		},
		{
			name:    "bad duration",
			jobs:    "    jobs:\n      - {name: a, schedule: \"* * * * *\", command: [x], timeout: soon}\n",
			wantErr: "timeout",
		},
		{
			name:    "missing command",
			jobs:    "    jobs:\n      - {name: a, schedule: \"* * * * *\"}\n",
			wantErr: "command",
		},
		{
			name:    "limits on exec mode",
			jobs:    "    jobs:\n      - {name: a, schedule: \"* * * * *\", command: [x], mode: exec, memory: 64Mi}\n",
			wantErr: "exec",
		},
		{
			name:    "unknown field",
			jobs:    "    jobs:\n      - {name: a, schedule: \"* * * * *\", command: [x], when: now}\n",
			wantErr: "when",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := Parse([]byte(base + tt.jobs))
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Parse: %v", err)
				}
				jobs := s.Services["web"].Jobs
				if len(jobs) != 1 || jobs[0].Enabled == nil || *jobs[0].Enabled || jobs[0].Retries != 2 || jobs[0].CatchUp != JobCatchUpRunOnce {
					t.Errorf("jobs = %+v, want the full job decoded", jobs)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Parse error = %v, want it to mention %q", err, tt.wantErr)
			}
		})
	}
}
