package compose

import (
	"fmt"
	"strings"
)

// UnresolvedVar is one SERVICE_ token ResolveMagicVars couldn't resolve:
// not a generatable kind, and no bash-style ${...:-default} fallback.
type UnresolvedVar struct {
	Service string
	EnvKey  string
	Token   string
}

func (u UnresolvedVar) String() string {
	return fmt.Sprintf("service %q: env %q: %s", u.Service, u.EnvKey, u.Token)
}

// ResolveMagicVars scans every service's environment for SERVICE_
// placeholders, mutating f in place: a token with a bash-style default
// substitutes that default as a literal value; a generatable token
// (PASSWORD/USER/BASE64/HEX/REALBASE64) is removed from Environment
// entirely and returned via secretEnv instead, since its real value
// belongs in secret storage, not a literal desired-state column.
//
// generate is called once per unique (kind, key) pair even when
// referenced by several services, so they all resolve to the same
// value (e.g. an app service's DB_PASSWORD and its sibling postgres
// service's own POSTGRES_PASSWORD both referencing SERVICE_PASSWORD_DB).
// persist is then called once per (service, env key) that ended up
// secret-backed, since secret storage is keyed per real service, not
// per magic-var key: the caller is expected to write that same value
// into whichever per-service secret store its later container-create
// step reads from.
//
// A magic-var token embedded inside a larger literal string (not the
// entire env value) can only be resolved via its own default; a
// generatable or default-less token in that position is always
// unresolved, since a generated/secret value can't be spliced into a
// literal string.
func ResolveMagicVars(
	f *File,
	generate func(kind, key string, length int) (string, error),
	persist func(serviceKey, envKey, value string) error,
) (secretEnv map[string][]string, unresolved []UnresolvedVar, err error) {
	secretEnv = make(map[string][]string)
	generated := make(map[string]string)

	for _, svcKey := range sortedServiceNames(f) {
		svc := f.Services[svcKey]
		if len(svc.Environment) == 0 {
			continue
		}
		newEnv := make(Environment, len(svc.Environment))
		for envKey, value := range svc.Environment {
			vars := FindMagicVars(value)
			if len(vars) == 0 {
				newEnv[envKey] = value
				continue
			}

			if len(vars) == 1 && vars[0].Token == value {
				v := vars[0]
				if v.Generatable {
					val, genErr := resolveGenerated(generated, v, generate)
					if genErr != nil {
						return nil, nil, fmt.Errorf("service %q: env %q: %w", svcKey, envKey, genErr)
					}
					if err := persist(svcKey, envKey, val); err != nil {
						return nil, nil, fmt.Errorf("service %q: env %q: persist secret: %w", svcKey, envKey, err)
					}
					secretEnv[svcKey] = append(secretEnv[svcKey], envKey)
					continue
				}
				if v.HasDefault {
					newEnv[envKey] = v.Default
					continue
				}
				unresolved = append(unresolved, UnresolvedVar{Service: svcKey, EnvKey: envKey, Token: v.Token})
				continue
			}

			resolved := value
			for _, v := range vars {
				if !v.Generatable && v.HasDefault {
					resolved = strings.Replace(resolved, v.Token, v.Default, 1)
					continue
				}
				unresolved = append(unresolved, UnresolvedVar{Service: svcKey, EnvKey: envKey, Token: v.Token})
			}
			newEnv[envKey] = resolved
		}
		svc.Environment = newEnv
		f.Services[svcKey] = svc
	}
	return secretEnv, unresolved, nil
}

func resolveGenerated(cache map[string]string, v MagicVar, generate func(kind, key string, length int) (string, error)) (string, error) {
	cacheKey := v.Kind + ":" + v.Key
	if val, ok := cache[cacheKey]; ok {
		return val, nil
	}
	val, err := generate(v.Kind, v.Key, v.Length)
	if err != nil {
		return "", fmt.Errorf("generate %s: %w", v.Token, err)
	}
	cache[cacheKey] = val
	return val, nil
}
