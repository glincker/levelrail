package authengine

import (
	"encoding/base64"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func unsignedToken(t *testing.T, claims map[string]any) string {
	t.Helper()
	b, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	return "e30." + base64.RawURLEncoding.EncodeToString(b) + ".sig"
}

func TestReadIDTokenClaims(t *testing.T) {
	long := strings.Repeat("g", maxClaimGroupLen+1)
	tests := []struct {
		name   string
		claim  string
		claims map[string]any
		want   IDTokenClaims
	}{
		{"list", "groups", map[string]any{"groups": []any{"a", " b ", "a", ""}}, IDTokenClaims{Groups: []string{"a", "b"}, GroupsFound: true}},
		{"bare string is one group", "groups", map[string]any{"groups": "a,b"}, IDTokenClaims{Groups: []string{"a,b"}, GroupsFound: true}},
		{"nested path", "realm_access.roles", map[string]any{"realm_access": map[string]any{"roles": []any{"x"}}}, IDTokenClaims{Groups: []string{"x"}, GroupsFound: true}},
		{"literal key with a colon", "cognito:groups", map[string]any{"cognito:groups": []any{"x"}}, IDTokenClaims{Groups: []string{"x"}, GroupsFound: true}},
		{"missing claim names look-alikes", "groups", map[string]any{"roles": []any{"x"}, "name": "n"}, IDTokenClaims{GroupLikeClaims: []string{"roles"}}},
		{"non string members and oversize names are dropped", "groups", map[string]any{"groups": []any{1, long, "ok"}}, IDTokenClaims{Groups: []string{"ok"}, GroupsFound: true}},
		{"empty claim name finds nothing", "", map[string]any{"groups": []any{"a"}}, IDTokenClaims{GroupLikeClaims: []string{"groups"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ReadIDTokenClaims(unsignedToken(t, tc.claims), tc.claim)
			if len(got.Groups) == 0 {
				got.Groups = nil
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestReadIDTokenClaimsRefusesMalformedAndHugeTokens(t *testing.T) {
	for name, tok := range map[string]string{
		"empty": "", "two parts": "a.b", "bad base64": "a.!!!.c", "not json": "a." + base64.RawURLEncoding.EncodeToString([]byte("x")) + ".c",
		"huge": "a." + strings.Repeat("A", maxIDTokenBytes) + ".c",
	} {
		if got := ReadIDTokenClaims(tok, "groups"); got.GroupsFound || len(got.Groups) != 0 {
			t.Errorf("%s: read claims %+v", name, got)
		}
	}
}

func TestReadIDTokenClaimsBoundsGroupCount(t *testing.T) {
	many := make([]any, 0, maxClaimGroups+50)
	for i := range maxClaimGroups + 50 {
		many = append(many, "group-"+strconv.Itoa(i))
	}
	got := ReadIDTokenClaims(unsignedToken(t, map[string]any{"groups": many}), "groups")
	if len(got.Groups) != maxClaimGroups {
		t.Errorf("read %d groups, want the cap of %d", len(got.Groups), maxClaimGroups)
	}
}

func TestEmailDomainAllowed(t *testing.T) {
	tests := []struct {
		email   string
		domains []string
		want    bool
	}{
		{"a@x.test", nil, true},
		{"a@X.test", []string{"x.test"}, true},
		{"a@x.test", []string{"@x.test"}, true},
		{"a@sub.x.test", []string{"x.test"}, false},
		{"a@evil.test@x.test", []string{"x.test"}, true},
		{"nodomain", []string{"x.test"}, false},
	}
	for _, tc := range tests {
		if got := EmailDomainAllowed(tc.email, tc.domains); got != tc.want {
			t.Errorf("EmailDomainAllowed(%q, %v) = %v, want %v", tc.email, tc.domains, got, tc.want)
		}
	}
}
