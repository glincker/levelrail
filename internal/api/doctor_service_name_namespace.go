package api

import (
	"context"
	"fmt"
	"strings"
)

type serviceNameAuditor interface {
	ServiceNamesBreakingNamespaceRule(ctx context.Context) ([]string, error)
}

// doctorCheckServiceNameNamespace lists apps whose names could alias an internal secret namespace (read only).
func (rt *Router) doctorCheckServiceNameNamespace(ctx context.Context) doctorCheckResource {
	const code, name = "service_name_namespace", "App name namespaces"
	a, ok := rt.apps.(serviceNameAuditor)
	if !ok {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "App names could not be audited on this store."}
	}
	bad, err := a.ServiceNamesBreakingNamespaceRule(ctx)
	if err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: fmt.Sprintf("App names could not be read: %s", err)}
	}
	if len(bad) == 0 {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: "No app name can alias an internal secret namespace."}
	}
	return doctorCheckResource{
		Code: code, Name: name, Status: doctorStatusWarn,
		Message: fmt.Sprintf("%d app name(s) contain a slash, backslash or control character and can alias an internal secret namespace: %s", len(bad), strings.Join(bad, ", ")),
		Fix:     "Review these apps, copy what you need into a correctly named app and delete the old one.",
	}
}
