package iac

import (
	"strings"
	"testing"
)

const composeDoc = "version: 1\nkind: App\nmetadata: {name: shop}\nspec:\n  project: web\n  compose:\n    path: compose/shop.yaml\n    profiles: [web, api]\n    env:\n      LOG_LEVEL: info\n"

func enableCompose(t *testing.T) {
	t.Helper()
	SetComposeSource(func() ComposeLimits {
		return ComposeLimits{Enabled: true, MaxFileBytes: 64, MaxFiles: 2, MaxTotalBytes: 100}
	})
	t.Cleanup(func() { SetComposeSource(func() ComposeLimits { return ComposeLimits{} }) })
}

func buildCompose(t *testing.T, doc string, att map[string][]byte) ([]*Resource, []Issue) {
	t.Helper()
	docs, issues := ParseDocuments([]Source{{Name: "a.yaml", Data: []byte(doc)}})
	if len(issues) > 0 {
		t.Fatal(issues)
	}
	return Build(docs, Options{Attachments: att})
}

func TestComposeRefBuild(t *testing.T) {
	enableCompose(t)
	att := map[string][]byte{"compose/shop.yaml": []byte("services: {web: {image: nginx}}\n")}
	res, issues := buildCompose(t, composeDoc, att)
	if len(issues) > 0 || len(res) != 1 || res[0].ComposeDigest == "" {
		t.Fatalf("res=%v issues=%v", res, issues)
	}
	a := res[0].ComposeDigest
	att["compose/shop.yaml"] = []byte("services: {a: {image: nginx}}\n")
	res, _ = buildCompose(t, composeDoc, att)
	if res[0].ComposeDigest == a {
		t.Fatal("a byte change must change the digest")
	}
	swapped := strings.Replace(composeDoc, "[web, api]", "[api, web]", 1)
	att["compose/shop.yaml"] = []byte("services: {web: {image: nginx}}\n")
	res, _ = buildCompose(t, swapped, att)
	if res[0].ComposeDigest != a {
		t.Fatal("profile order must not change the digest")
	}

	cases := map[string]struct {
		doc  string
		att  map[string][]byte
		code string
	}{
		"missing":   {composeDoc, nil, CodeComposeRefMissing},
		"conflict":  {composeDoc + "  service:\n    build: {type: image, image: x}\n    port: 80\n", att, CodeComposeModeConflict},
		"dotdot":    {strings.Replace(composeDoc, "compose/shop.yaml", "../shop.yaml", 1), att, CodeComposePathInvalid},
		"absolute":  {strings.Replace(composeDoc, "compose/shop.yaml", "/etc/shop.yaml", 1), att, CodeComposePathInvalid},
		"backslash": {strings.Replace(composeDoc, "compose/shop.yaml", `a\\b.yaml`, 1), att, CodeComposePathInvalid},
		"toolarge":  {composeDoc, map[string][]byte{"compose/shop.yaml": make([]byte, 65)}, CodeComposeTooLarge},
		"toomany":   {composeDoc, map[string][]byte{"compose/shop.yaml": nil, "a.yaml": nil, "b.yaml": nil}, CodeComposeTooManyFiles},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, issues := buildCompose(t, c.doc, c.att)
			for _, i := range issues {
				if i.Code == c.code {
					return
				}
			}
			t.Fatalf("want code %s, got %+v", c.code, issues)
		})
	}
	if _, issues := buildCompose(t, strings.Replace(composeDoc, "service", "x", 1)+"    service: web\n", att); len(issues) == 0 {
		t.Fatal("the service selector must be refused")
	}
}

func TestComposeRefSplit(t *testing.T) {
	files := []Source{
		{Name: "app.yaml", Data: []byte(composeDoc)},
		{Name: "compose/shop.yaml", Data: []byte("services:\n  web: {image: nginx}\n")},
		{Name: "other.yaml", Data: []byte("version: 1\nkind: Project\nmetadata: {name: web}\nspec: {}\n")},
	}
	docs, att, issues := SplitComposeRefs(files)
	if len(docs) != 2 || len(att) != 1 || len(issues) != 0 {
		t.Fatalf("docs=%d att=%d issues=%v", len(docs), len(att), issues)
	}
	if _, ok := att["compose/shop.yaml"]; !ok {
		t.Fatal("the referenced file must become an attachment")
	}
}

func TestComposeRefHashKeepsExistingPlans(t *testing.T) {
	SetPlanHashKey([]byte("k"))
	base := []Change{{Kind: KindApp, Name: "a", Action: ActionCreate, Fields: []FieldChange{{Path: "image", New: "x"}}}}
	h1 := planHashV2("s", false, base, nil)
	withEmpty := []Change{base[0]}
	withEmpty[0].ComposeDigest = ""
	if h1 != planHashV2("s", false, withEmpty, nil) {
		t.Fatal("an empty compose digest must not change the hash")
	}
	withEmpty[0].ComposeDigest = "abc"
	if h1 == planHashV2("s", false, withEmpty, nil) {
		t.Fatal("a compose digest must change the hash")
	}
}
