package compose

import "testing"

func TestInlineConfigsBecomeFiles(t *testing.T) {
	f, err := Parse([]byte(`services:
  app:
    image: nginx:1.27
    configs:
      - source: site
        target: /etc/app/site.conf
        mode: 0440
configs:
  site:
    content: |
      listen 80;
`))
	if err != nil {
		t.Fatal(err)
	}
	ds, _, err := ToDesiredServices("a", f)
	if err != nil {
		t.Fatal(err)
	}
	got := ds[0].Files
	if len(got) != 1 || got[0].Path != "/etc/app/site.conf" || got[0].Mode != "0440" || got[0].Content != "listen 80;\n" {
		t.Fatalf("files = %+v", got)
	}
}

func TestFileConfigsStayUnsupported(t *testing.T) {
	f, err := Parse([]byte("services:\n  app:\n    image: nginx\n    configs: [site]\nconfigs:\n  site:\n    file: ./site.conf\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Validate(); err == nil {
		t.Fatal("want error for file: config")
	}
}
