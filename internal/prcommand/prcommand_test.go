package prcommand

import (
	"errors"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		body string
		want Command
		err  error
	}{
		{"redeploy", "/levelrail redeploy", Redeploy, nil},
		{"rollback", "/levelrail rollback", Rollback, nil},
		{"status", "/levelrail status", Status, nil},
		{"preview redeploy", "/levelrail preview redeploy", PreviewRedeploy, nil},
		{"upper case", "/LEVELRAIL REDEPLOY", Redeploy, nil},
		{"mixed case", "/LevelRail Preview Redeploy", PreviewRedeploy, nil},
		{"extra spaces", "/levelrail   preview    redeploy  ", PreviewRedeploy, nil},
		{"crlf", "looks good\r\n/levelrail status\r\n", Status, nil},
		{"after prose line", "ship it\n/levelrail redeploy\nthanks", Redeploy, nil},
		{"same line prose before", "please /levelrail redeploy", "", nil},
		{"leading space", " /levelrail redeploy", "", nil},
		{"indented code", "    /levelrail redeploy", "", nil},
		{"tab indented", "\t/levelrail redeploy", "", nil},
		{"quoted reply", "> /levelrail redeploy", "", nil},
		{"nested quote", ">> /levelrail rollback", "", nil},
		{"backtick fence", "```\n/levelrail redeploy\n```", "", nil},
		{"tilde fence", "~~~sh\n/levelrail rollback\n~~~", "", nil},
		{"command after closed fence", "```\ncode\n```\n/levelrail status", Status, nil},
		{"unclosed fence hides rest", "```\n/levelrail redeploy", "", nil},
		{"inline code", "`/levelrail redeploy`", "", nil},
		{"two different commands", "/levelrail redeploy\n/levelrail rollback", "", ErrMultiple},
		{"same command twice", "/levelrail redeploy\n/levelrail redeploy", "", ErrMultiple},
		{"one real one quoted", "> /levelrail rollback\n/levelrail status", Status, nil},
		{"trailing arguments", "/levelrail redeploy now", "", nil},
		{"app name smuggled", "/levelrail redeploy other-app", "", nil},
		{"shell injection", "/levelrail redeploy; rm -rf /", "", nil},
		{"newline injection in verb", "/levelrail\nredeploy", "", nil},
		{"unknown verb", "/levelrail destroy", "", nil},
		{"help", "/levelrail help", Help, nil},
		{"help with argument", "/levelrail help redeploy", "", nil},
		{"preview alone", "/levelrail preview", "", nil},
		{"no space", "/levelrailredeploy", "", nil},
		{"prefix typo", "/levelrails redeploy", "", nil},
		{"html comment", "<!-- /levelrail redeploy -->", "", nil},
		{"markdown bullet", "- /levelrail redeploy", "", nil},
		{"zero width prefix", "\u200b/levelrail redeploy", "", nil},
		{"empty", "", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.body)
			if !errors.Is(err, tt.err) || got != tt.want {
				t.Fatalf("Parse(%q) = (%q, %v), want (%q, %v)", tt.body, got, err, tt.want, tt.err)
			}
		})
	}
}
