package compose

import (
	"fmt"
	"strings"
)

// resolveFileMagicVars fills generated values into a secret file's body, so a
// template can ship a config that holds a generated token without the value
// ever sitting in compose text. Plain files only report unresolved prompts.
func resolveFileMagicVars(svcKey string, svc *Service, generated map[string]string, generate func(kind, key string, length int) (string, error)) ([]UnresolvedVar, error) {
	var unresolved []UnresolvedVar
	for i, file := range svc.Files {
		if file.Content == RedactedSecretMarker {
			continue
		}
		body := file.Content
		for _, v := range FindMagicVars(file.Content) {
			switch {
			case v.Kind == "PROMPT":
				unresolved = append(unresolved, UnresolvedVar{Service: svcKey, EnvKey: file.Path, Token: v.Token})
			case !file.Secret:
			case v.Generatable:
				val, err := resolveGenerated(generated, v, generate)
				if err != nil {
					return nil, fmt.Errorf("service %q: file %q: %w", svcKey, file.Path, err)
				}
				body = strings.Replace(body, v.Token, val, 1)
			case v.HasDefault:
				body = strings.Replace(body, v.Token, v.Default, 1)
			}
		}
		svc.Files[i].Content = body
	}
	return unresolved, nil
}
