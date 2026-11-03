package api

import (
	"net/http"
	"testing"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

func TestPasswordPolicyCheck(t *testing.T) {
	cases := []struct {
		name string
		p    passwordPolicy
		pw   string
		want string
	}{
		{"default accepts 8 chars", passwordPolicy{8, 0}, "abcdefgh", ""},
		{"default refuses 7 chars", passwordPolicy{8, 0}, "abcdefg", "password must be at least 8 characters"},
		{"longer minimum", passwordPolicy{14, 0}, "abcdefghijklm", "password must be at least 14 characters"},
		{"one class refused at two", passwordPolicy{8, 2}, "abcdefghij", "password must mix at least 2 of: lowercase letters, uppercase letters, digits, symbols"},
		{"two classes pass", passwordPolicy{8, 2}, "abcdefgh1", ""},
		{"four classes pass", passwordPolicy{8, 4}, "Abcdef1!", ""},
		{"three of four refused", passwordPolicy{8, 4}, "Abcdefg1", "password must mix at least 4 of: lowercase letters, uppercase letters, digits, symbols"},
		{"both problems named", passwordPolicy{12, 3}, "abc", "password must be at least 12 characters and mix at least 3 of: lowercase letters, uppercase letters, digits, symbols"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.p.Check("password", tc.pw); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPasswordPolicy_SettingsApplyLiveAndFloor(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := t.Context()
	if p := rt.passwordPolicyNow(ctx); p.MinLength != 8 || p.Classes != 0 {
		t.Fatalf("default policy = %+v, want 8 and 0", p)
	}
	puts := []struct {
		key, val string
		want     int
	}{
		{runtimesettings.KeyAuthPasswordMinLength, "7", http.StatusBadRequest},
		{runtimesettings.KeyAuthPasswordMinLength, "12", http.StatusOK},
		{runtimesettings.KeyAuthPasswordRequireClasses, "5", http.StatusBadRequest},
		{runtimesettings.KeyAuthPasswordRequireClasses, "3", http.StatusOK},
	}
	for _, c := range puts {
		if rec := rsDo(t, rt, cookie, http.MethodPut, rsBase+"/"+c.key, `{"value":"`+c.val+`"}`); rec.Code != c.want {
			t.Fatalf("%s=%s: %d, want %d", c.key, c.val, rec.Code, c.want)
		}
	}
	if p := rt.passwordPolicyNow(ctx); p.MinLength != 12 || p.Classes != 3 {
		t.Fatalf("live policy = %+v, want 12 and 3", p)
	}
}
