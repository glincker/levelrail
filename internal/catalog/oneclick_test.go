package catalog

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestConvertOneClickGolden(t *testing.T) {
	in, err := os.ReadFile("testdata/oneclick_basic.json")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/oneclick_basic.compose.golden")
	if err != nil {
		t.Fatal(err)
	}
	res, err := ConvertOneClick(in)
	if err != nil {
		t.Fatal(err)
	}
	if res.ComposeYAML != string(want) {
		t.Fatalf("compose mismatch:\n%s", res.ComposeYAML)
	}
	if res.Name != "Example" || !reflect.DeepEqual(res.Variables, []string{"DB_PASS"}) {
		t.Fatalf("meta: %+v", res)
	}
	joined := strings.Join(res.Warnings, "|")
	if !strings.Contains(joined, `service "web": key "vendorExtra" dropped`) {
		t.Fatalf("warnings: %v", res.Warnings)
	}
}

func TestConvertOneClickRejects(t *testing.T) {
	cases := map[string]string{
		"not json":   `{`,
		"no service": `{"services":{}}`,
		"no image":   `{"services":{"web":{}}}`,
		"bad name":   `{"services":{"$$cap_x":{"image":"a"}}}`,
	}
	for name, in := range cases {
		if _, err := ConvertOneClick([]byte(in)); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
	if _, err := ConvertOneClick(make([]byte, MaxOneClickBytes+1)); err == nil {
		t.Error("oversize: want error")
	}
}

func TestConvertOneClickUndeclaredVariableWarns(t *testing.T) {
	res, err := ConvertOneClick([]byte(`{"services":{"a":{"image":"i:$$cap_tag"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "no declared default") {
		t.Fatalf("warnings: %v", res.Warnings)
	}
}
