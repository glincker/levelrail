package provision

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestBootstrapScriptAppendedAfterBuiltIn(t *testing.T) {
	base := "#cloud-init\nwrite_files:\n  - path: /etc/a.env\n    content: |\n      X=1\nruncmd:\n  - systemctl enable --now docker\n"
	got, err := AppendBootstrapScript(base, "agent", "#!/bin/sh\necho 'hi: there'\n")
	if err != nil {
		t.Fatal(err)
	}
	enc := base64.StdEncoding.EncodeToString([]byte("#!/bin/sh\necho 'hi: there'\n"))
	if !strings.Contains(got, "content: "+enc) {
		t.Fatalf("encoded body missing:\n%s", got)
	}
	if strings.Index(got, "path: /usr/local/sbin/agent-bootstrap-script") > strings.Index(got, "runcmd:") {
		t.Fatal("write_files entry must precede runcmd")
	}
	if !strings.HasSuffix(got, "  - /usr/local/sbin/agent-bootstrap-script\n") {
		t.Fatalf("script must run last:\n%s", got)
	}
	if same, _ := AppendBootstrapScript(base, "agent", ""); same != base {
		t.Fatal("empty body must leave user data unchanged")
	}
}
