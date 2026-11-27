package compose

import (
	"reflect"
	"strings"
	"testing"
)

const nodeRefBase = "services:\n  web:\n    image: nginx:1.27\n"

func TestNodeRefParsesAndValidates(t *testing.T) {
	f, err := Parse([]byte(nodeRefBase + "    x-levelrail-node: oci1\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := f.Services["web"].Node; got != "oci1" {
		t.Fatalf("node = %q, want oci1", got)
	}
	sel, err := NodeRefSelector("zone=a,gpu=true")
	if err != nil || sel["zone"] != "a" || sel["gpu"] != "true" {
		t.Fatalf("selector = %v, %v", sel, err)
	}
	bad, err := Parse([]byte(nodeRefBase + "    x-levelrail-node: \"bad=\"\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), "x-levelrail-node") {
		t.Fatalf("want a named x-levelrail-node error, got %v", err)
	}
}

func TestNodeRefAbsentLeavesTranslationUnchanged(t *testing.T) {
	plain, err := Parse([]byte(nodeRefBase))
	if err != nil {
		t.Fatal(err)
	}
	pinned, err := Parse([]byte(nodeRefBase + "    x-levelrail-node: oci1\n"))
	if err != nil {
		t.Fatal(err)
	}
	a, _, err := ToDesiredServices("app", plain)
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := ToDesiredServices("app", pinned)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("the node key must not change the translated services:\n%+v\n%+v", a, b)
	}
}
