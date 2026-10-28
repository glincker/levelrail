package compose

import (
	"strings"
	"testing"
)

func TestRestartPolicy_StrictOnFailureCount(t *testing.T) {
	tests := []struct {
		restart     string
		wantPolicy  string
		wantOneShot bool
		wantLevel   NoticeLevel
		wantInMsg   string
	}{
		{restart: `"on-failure:0"`, wantPolicy: "on-failure:0", wantOneShot: true, wantLevel: NoticeLevelNote, wantInMsg: "N is accepted and ignored"},
		{restart: `'on-failure:5'`, wantPolicy: "on-failure:5", wantOneShot: true, wantLevel: NoticeLevelNote, wantInMsg: "N is accepted and ignored"},
		{restart: "On-Failure:3", wantPolicy: "on-failure:3", wantOneShot: true, wantLevel: NoticeLevelNote, wantInMsg: "N is accepted and ignored"},
		{restart: `"NO"`, wantPolicy: "no", wantOneShot: true, wantLevel: NoticeLevelNote, wantInMsg: "runs it to completion"},
		{restart: "ON-FAILURE", wantPolicy: "on-failure", wantOneShot: true, wantLevel: NoticeLevelNote, wantInMsg: "runs it to completion"},
		{restart: "on-failure:abc", wantLevel: NoticeLevelWarning, wantInMsg: "ignored"},
		{restart: `"on-failure:"`, wantLevel: NoticeLevelWarning, wantInMsg: "ignored"},
		{restart: `"on-failure:-1"`, wantLevel: NoticeLevelWarning, wantInMsg: "ignored"},
		{restart: `"on-failure:+2"`, wantLevel: NoticeLevelWarning, wantInMsg: "ignored"},
		{restart: `"on-failure:1.5"`, wantLevel: NoticeLevelWarning, wantInMsg: "ignored"},
		{restart: "sometimes", wantLevel: NoticeLevelWarning, wantInMsg: "ignored"},
	}
	for _, tt := range tests {
		t.Run(tt.restart, func(t *testing.T) {
			f, err := Parse([]byte("services:\n  migrate:\n    image: app:v1\n    restart: " + tt.restart + "\n"))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			svcs, _, err := ToDesiredServices("app", f)
			if err != nil {
				t.Fatalf("ToDesiredServices: %v", err)
			}
			got := svcs[0]
			if got.RunsToCompletion() != tt.wantOneShot {
				t.Fatalf("RunsToCompletion = %v, want %v (Health %+v)", got.RunsToCompletion(), tt.wantOneShot, got.Health)
			}
			if tt.wantOneShot && got.Health.Restart != tt.wantPolicy {
				t.Fatalf("Health.Restart = %q, want %q", got.Health.Restart, tt.wantPolicy)
			}
			if !tt.wantOneShot && got.Health != nil {
				t.Fatalf("Health = %+v, want nil for an ignored restart value", got.Health)
			}
			notices := f.Notices()
			if len(notices) != 1 {
				t.Fatalf("Notices = %+v, want exactly one", notices)
			}
			if notices[0].Level != tt.wantLevel || !strings.Contains(notices[0].Message, tt.wantInMsg) {
				t.Fatalf("notice = %+v, want level %v containing %q", notices[0], tt.wantLevel, tt.wantInMsg)
			}
		})
	}
}
