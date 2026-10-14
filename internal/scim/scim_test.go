package scim

import (
	"errors"
	"reflect"
	"testing"
)

func TestParseFilter(t *testing.T) {
	for _, tc := range []struct {
		in      string
		want    *Filter
		wantErr bool
	}{
		{"", nil, false},
		{`userName eq "ada@example.com"`, &Filter{"userName", "ada@example.com"}, false},
		{`USERNAME EQ "ada@example.com"`, &Filter{"userName", "ada@example.com"}, false},
		{`externalId eq "00u1"`, &Filter{"externalId", "00u1"}, false},
		{`userName eq "a\"b"`, nil, true},
		{`userName eq "a" and active eq "true"`, nil, true},
		{`userName co "ada"`, nil, true},
		{`password eq "x"`, nil, true},
		{`userName eq ada`, nil, true},
		{`userName`, nil, true},
	} {
		got, err := ParseFilter(tc.in, "userName", "externalId")
		if (err != nil) != tc.wantErr {
			t.Fatalf("%q: err %v", tc.in, err)
		}
		if tc.wantErr && !errors.Is(err, ErrInvalidFilter) {
			t.Fatalf("%q: err %v is not ErrInvalidFilter", tc.in, err)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%q: got %+v want %+v", tc.in, got, tc.want)
		}
	}
}

func strp(s string) *string { return &s }
func boolp(b bool) *bool    { return &b }

func TestParseUserPatch(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want UserChange
	}{
		{"okta deactivate", `{"Operations":[{"op":"replace","value":{"active":false}}]}`, UserChange{Active: boolp(false)}},
		{"entra deactivate string", `{"Operations":[{"op":"Replace","path":"active","value":"False"}]}`, UserChange{Active: boolp(false)}},
		{"entra email", `{"Operations":[{"op":"Add","path":"emails[type eq \"work\"].value","value":"a@x.io"}]}`, UserChange{Email: strp("a@x.io")}},
		{"name parts", `{"Operations":[{"op":"replace","path":"name.givenName","value":"Ada"},{"op":"replace","path":"name.familyName","value":"L"}]}`, UserChange{GivenName: strp("Ada"), FamilyName: strp("L")}},
		{"unknown ignored", `{"Operations":[{"op":"replace","path":"title","value":"CTO"}]}`, UserChange{}},
		{"remove display name", `{"Operations":[{"op":"remove","path":"displayName"}]}`, UserChange{DisplayName: strp("")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseUserPatch([]byte(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %+v want %+v", got, tc.want)
			}
		})
	}
	for name, body := range map[string]string{
		"not json":       `{`,
		"no operations":  `{"Operations":[]}`,
		"bad op":         `{"Operations":[{"op":"move","path":"active","value":true}]}`,
		"active string":  `{"Operations":[{"op":"replace","path":"active","value":"maybe"}]}`,
		"remove active":  `{"Operations":[{"op":"remove","path":"active"}]}`,
		"remove no path": `{"Operations":[{"op":"remove"}]}`,
		"value not obj":  `{"Operations":[{"op":"replace","value":"x"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseUserPatch([]byte(body)); !errors.Is(err, ErrInvalidPatch) {
				t.Fatalf("err %v", err)
			}
		})
	}
}

func TestParseGroupPatch(t *testing.T) {
	got, err := ParseGroupPatch([]byte(`{"Operations":[
		{"op":"add","path":"members","value":[{"value":"u1"},{"value":"u2"}]},
		{"op":"remove","path":"members[value eq \"u1\"]"},
		{"op":"replace","value":{"id":"g1","displayName":"Ops"}},
		{"op":"remove","path":"members"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	want := GroupChange{DisplayName: strp("Ops"), Members: []MemberOp{
		{Kind: MembersAdd, IDs: []string{"u1", "u2"}},
		{Kind: MembersRemove, IDs: []string{"u1"}},
		{Kind: MembersReplace, IDs: []string{}},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v want %+v", got, want)
	}
	for name, body := range map[string]string{
		"empty member":   `{"Operations":[{"op":"add","path":"members","value":[{"value":""}]}]}`,
		"add no value":   `{"Operations":[{"op":"add","path":"members"}]}`,
		"bad path":       `{"Operations":[{"op":"replace","path":"owner","value":"x"}]}`,
		"bad filter":     `{"Operations":[{"op":"remove","path":"members[display eq \"x\"]"}]}`,
		"filter add":     `{"Operations":[{"op":"add","path":"members[value eq \"x\"]"}]}`,
		"remove display": `{"Operations":[{"op":"remove","path":"displayName"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseGroupPatch([]byte(body)); !errors.Is(err, ErrInvalidPatch) {
				t.Fatalf("err %v", err)
			}
		})
	}
}

func TestPageAndBool(t *testing.T) {
	if s, n := Page(0, 0, 100); s != 1 || n != 100 {
		t.Fatalf("Page = %d %d", s, n)
	}
	if s, n := Page(5, 500, 100); s != 5 || n != 100 {
		t.Fatalf("Page = %d %d", s, n)
	}
	for raw, want := range map[string]bool{`true`: true, `"True"`: true, `"false"`: false, `false`: false} {
		if got, err := ParseBool([]byte(raw)); err != nil || got != want {
			t.Fatalf("ParseBool(%s) = %v %v", raw, got, err)
		}
	}
	if _, err := ParseBool([]byte(`1`)); err == nil {
		t.Fatal("ParseBool accepted 1")
	}
	if got := (User{UserName: "x", Emails: []Email{{Value: "a@x"}, {Value: "b@x", Primary: true}}}).PrimaryEmail(); got != "b@x" {
		t.Fatalf("PrimaryEmail = %q", got)
	}
}
