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

// ResolveMagicVars scans every service's environment and command for
// SERVICE_ placeholders, mutating f in place: a token with a bash-style
// default substitutes that default as a literal value; a generatable
// token (PASSWORD/USER/BASE64/HEX/REALBASE64) found in Environment is
// removed from it entirely and returned via secretEnv instead, since its
// real value belongs in secret storage, not a literal desired-state
// column. Command has no equivalent secret-storage indirection (a
// Command entry can't be swapped for a decrypted-at-create-time secret
// the way an env var can), so a generatable token found there is always
// reported as unresolved instead, same as a default-less non-generatable
// token.
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
// A magic-var token embedded inside a larger string (not the entire env
// value, e.g. a composite DATABASE_URL) can still splice in a
// generated value: the whole assembled string is then secret-backed
// (persisted and added to secretEnv), the same as a whole-value
// generatable token, since it now contains one. A default-less,
// non-generatable token in that position is unresolved.
func ResolveMagicVars(
	f *File,
	generate func(kind, key string, length int) (string, error),
	persist func(serviceKey, envKey, value string) error,
) (secretEnv map[string][]string, unresolved []UnresolvedVar, err error) {
	secretEnv = make(map[string][]string)
	generated := make(map[string]string)

	for _, svcKey := range sortedServiceNames(f) {
		svc := f.Services[svcKey]
		changed := false

		if len(svc.Command) > 0 {
			newCommand := make(Command, len(svc.Command))
			for i, entry := range svc.Command {
				resolved, entryUnresolved := resolveCommandEntry(f, svcKey, i, entry)
				newCommand[i] = resolved
				unresolved = append(unresolved, entryUnresolved...)
			}
			svc.Command = newCommand
			changed = true
		}

		if len(svc.Environment) == 0 {
			if changed {
				f.Services[svcKey] = svc
			}
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
				if v.Kind == "FQDN" {
					if url, ok := resolveFQDN(f, svcKey); ok {
						newEnv[envKey] = url
						continue
					}
				}
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
			hasSecret := false
			for _, v := range vars {
				if v.Kind == "FQDN" {
					if url, ok := resolveFQDN(f, svcKey); ok {
						resolved = strings.Replace(resolved, v.Token, url, 1)
						continue
					}
				}
				if v.Generatable {
					val, genErr := resolveGenerated(generated, v, generate)
					if genErr != nil {
						return nil, nil, fmt.Errorf("service %q: env %q: %w", svcKey, envKey, genErr)
					}
					resolved = strings.Replace(resolved, v.Token, val, 1)
					hasSecret = true
					continue
				}
				if v.HasDefault {
					resolved = strings.Replace(resolved, v.Token, v.Default, 1)
					continue
				}
				unresolved = append(unresolved, UnresolvedVar{Service: svcKey, EnvKey: envKey, Token: v.Token})
			}
			if hasSecret {
				if err := persist(svcKey, envKey, resolved); err != nil {
					return nil, nil, fmt.Errorf("service %q: env %q: persist secret: %w", svcKey, envKey, err)
				}
				secretEnv[svcKey] = append(secretEnv[svcKey], envKey)
				continue
			}
			newEnv[envKey] = resolved
		}
		svc.Environment = newEnv
		f.Services[svcKey] = svc
	}
	return secretEnv, unresolved, nil
}

// resolveFQDN resolves ${SERVICE_FQDN_*} to the referencing service's
// own assigned domain (x-levelrail-domains), ignoring the token's own
// key suffix: FQDN almost always means "my own public URL," and that's
// a per-service fact, not a value shared across services the way a
// generated secret's (kind, key) is.
func resolveFQDN(f *File, svcKey string) (string, bool) {
	domain, ok := f.Domains[svcKey]
	if !ok || domain == "" {
		return "", false
	}
	return "https://" + domain, true
}

// resolveCommandEntry resolves every magic var in one Command entry the
// same way an embedded Environment token resolves (FQDN and
// bash-default tokens substitute in place), except a generatable-kind
// token: Command has no secret-storage indirection to land a generated
// value in (see ResolveMagicVars), so it's reported as unresolved
// instead of generated.
func resolveCommandEntry(f *File, svcKey string, index int, entry string) (string, []UnresolvedVar) {
	vars := FindMagicVars(entry)
	if len(vars) == 0 {
		return entry, nil
	}

	location := fmt.Sprintf("command[%d]", index)
	resolved := entry
	var unresolved []UnresolvedVar
	for _, v := range vars {
		if v.Kind == "FQDN" {
			if url, ok := resolveFQDN(f, svcKey); ok {
				resolved = strings.Replace(resolved, v.Token, url, 1)
				continue
			}
		}
		if v.Generatable {
			unresolved = append(unresolved, UnresolvedVar{Service: svcKey, EnvKey: location, Token: v.Token})
			continue
		}
		if v.HasDefault {
			resolved = strings.Replace(resolved, v.Token, v.Default, 1)
			continue
		}
		unresolved = append(unresolved, UnresolvedVar{Service: svcKey, EnvKey: location, Token: v.Token})
	}
	return resolved, unresolved
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
