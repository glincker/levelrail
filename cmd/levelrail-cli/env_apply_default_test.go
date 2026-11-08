package main

import "testing"

func TestEnvSetApplyOnSetSetting(t *testing.T) {
	pending := map[string]any{"pending": true, "changes": []map[string]any{{"kind": "env", "keys": []string{"A"}}}}
	cases := []struct {
		name    string
		setting string
		flag    string
		want    bool
	}{
		{"setting on restarts", "true", "", true},
		{"setting off keeps pending", "false", "", false},
		{"no-apply overrides setting", "true", "--no-apply", false},
		{"apply overrides setting", "false", "--apply", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bodies := map[string][]byte{}
			srv := routeServer(t, envSetRoutes(map[string]any{
				"GET /api/v1/apps/web/pending-changes":               pending,
				"GET /api/v1/settings/runtime/apps.env_apply_on_set": map[string]any{"key": "apps.env_apply_on_set", "value": tc.setting},
				"POST /api/v1/apps/web/apply-pending":                map[string]any{},
			}), bodies)
			args := []string{"env", "set", "web", "A=1"}
			if tc.flag != "" {
				args = append(args, tc.flag)
			}
			runCLIExpectOK(t, cliArgs(srv, args...))
			if _, got := bodies["POST /api/v1/apps/web/apply-pending"]; got != tc.want {
				t.Errorf("apply-pending called = %v, want %v", got, tc.want)
			}
		})
	}
}
