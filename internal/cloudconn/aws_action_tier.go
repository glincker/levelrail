package cloudconn

import (
	"path"
	"strings"
)

// CapabilityForAction names the tier whose statements grant an IAM action, so a doctor can
// tell the owner which tier to add instead of printing a bare action.
func CapabilityForAction(action string) (string, bool) {
	for _, c := range awsCapabilities {
		for _, st := range c.Statements {
			for _, a := range st.Actions {
				if strings.EqualFold(a, action) {
					return c.Name, true
				}
				if ok, _ := path.Match(strings.ToLower(a), strings.ToLower(action)); ok {
					return c.Name, true
				}
			}
		}
	}
	return "", false
}
