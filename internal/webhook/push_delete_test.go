package webhook

import "testing"

func TestParsePushEventDeleted(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{"github branch delete", `{"ref":"refs/heads/dev","after":"0000000000000000000000000000000000000000","before":"abc","deleted":true}`, true},
		{"gitlab zero after", `{"ref":"refs/heads/dev","after":"0000000000000000000000000000000000000000","before":"abc"}`, true},
		{"ordinary push", `{"ref":"refs/heads/dev","after":"abc123","before":"def"}`, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ev, err := ParsePushEvent([]byte(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			if ev.Deleted != tc.want {
				t.Fatalf("Deleted=%v want %v", ev.Deleted, tc.want)
			}
		})
	}
}
