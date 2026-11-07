package provision

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// AppendBootstrapScript adds an operator-saved script to rendered cloud-init user data. It is written
// base64-encoded so no script byte can break the YAML, and runs last, after the built-in bootstrap.
func AppendBootstrapScript(userData, agentName, body string) (string, error) {
	if body == "" {
		return userData, nil
	}
	const marker = "\nruncmd:\n"
	i := strings.Index(userData, marker)
	if i < 0 {
		return "", fmt.Errorf("provision: append bootstrap script: user data has no runcmd section")
	}
	path := "/usr/local/sbin/" + agentName + "-bootstrap-script"
	entry := fmt.Sprintf("  - path: %s\n    permissions: '0700'\n    encoding: b64\n    content: %s\n", path, base64.StdEncoding.EncodeToString([]byte(body)))
	out := userData[:i+1] + entry + userData[i+1:]
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out + "  - " + path + "\n", nil
}
