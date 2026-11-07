package compose

import "testing"

func TestEnvScopeMemberNames(t *testing.T) {
	if got := MemberAppName("blog", "db"); got != "blog-db" {
		t.Fatalf("member name = %q", got)
	}
	if key, ok := ServiceKeyOf("blog", "blog-db"); !ok || key != "db" {
		t.Fatalf("service key = %q %v", key, ok)
	}
	if _, ok := ServiceKeyOf("blog", "other-db"); ok {
		t.Fatal("foreign member matched")
	}
	if _, ok := ServiceKeyOf("blog", "blog-"); ok {
		t.Fatal("empty key matched")
	}
}
