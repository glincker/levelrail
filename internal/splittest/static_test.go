package splittest

import "testing"

func TestStaticImageRefRoundTrip(t *testing.T) {
	id, ok := ParseStaticImageRef(StaticImageRef("dep_abc"))
	if !ok || id != "dep_abc" {
		t.Fatalf("got %q %v", id, ok)
	}
	if _, ok := ParseStaticImageRef("nginx:1"); ok {
		t.Fatal("image parsed as static ref")
	}
	if _, ok := ParseStaticImageRef("static-deploy:"); ok {
		t.Fatal("empty id accepted")
	}
}
