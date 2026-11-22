package compose

import (
	"fmt"
	"maps"
	"slices"
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
// default substitutes that default as a literal value; a generatable.
func ResolveMagicVars(
	f *File,
	generate func(kind, key string, length int) (string, error),
	persist func(serviceKey, envKey, value string) error,
) (secretEnv map[string][]string, unresolved []UnresolvedVar, err error) {
	return resolveMagicVars(f, generate, persist, true)
}

// resolveMagicVars is ResolveMagicVars with the generic ${VAR:-default}
// pass switchable, so a test can compare against the SERVICE_-only result.
func resolveMagicVars(
	f *File,
	generate func(kind, key string, length int) (string, error),
	persist func(serviceKey, envKey, value string) error,
	interpolate bool,
) (secretEnv map[string][]string, unresolved []UnresolvedVar, err error) {
	secretEnv = make(map[string][]string)
	generated := make(map[string]string)

	for _, svcKey := range sortedServiceNames(f) {
		svc := f.Services[svcKey]
		changed := false

		if hasDollarEscape(svc) {
			if f.dollarEscaped == nil {
				f.dollarEscaped = map[string]bool{}
			}
			f.dollarEscaped[svcKey] = true
		}

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

		if len(svc.Entrypoint) > 0 {
			svc.Entrypoint = unescapeEntries(svc.Entrypoint)
			changed = true
		}
		if svc.Healthcheck != nil && len(svc.Healthcheck.Test) > 0 {
			hc := *svc.Healthcheck
			hc.Test = unescapeEntries(hc.Test)
			svc.Healthcheck = &hc
			changed = true
		}

		for _, field := range []struct {
			key string
			val *string
		}{{"user", &svc.User}, {"working_dir", &svc.WorkingDir}} {
			if *field.val == "" {
				continue
			}
			resolved, ferr := resolveScalarField(f, svcKey, field.key, *field.val, interpolate)
			if ferr != nil {
				return nil, nil, ferr
			}
			*field.val = resolved
			changed = true
		}

		fileUnresolved, ferr := resolveFileMagicVars(svcKey, &svc, generated, generate)
		if ferr != nil {
			return nil, nil, ferr
		}
		if len(svc.Files) > 0 {
			changed = true
		}
		unresolved = append(unresolved, fileUnresolved...)

		if len(svc.Environment) == 0 {
			if changed {
				f.Services[svcKey] = svc
			}
			continue
		}
		newEnv := make(Environment, len(svc.Environment))
		// Sorted so unresolved, warnings and persist order are deterministic.
		for _, envKey := range slices.Sorted(maps.Keys(svc.Environment)) {
			value := svc.Environment[envKey]
			if interpolate {
				var ierr error
				value, ierr = interpolateValue(value, f.InterpVars, func(w string) {
					f.InterpWarnings = append(f.InterpWarnings, fmt.Sprintf("service %q: env %q: %s", svcKey, envKey, w))
				})
				if ierr != nil {
					return nil, nil, fmt.Errorf("service %q: env %q: %w", svcKey, envKey, ierr)
				}
			}
			vars := FindMagicVars(value)
			if len(vars) == 0 {
				newEnv[envKey] = unescapeDollar(value)
				continue
			}

			if len(vars) == 1 && vars[0].Token == value {
				v := vars[0]
				if val, ok := resolveDomainVar(f, svcKey, v); ok {
					newEnv[envKey] = val
					continue
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
				if val, ok := resolveDomainVar(f, svcKey, v); ok {
					resolved = strings.Replace(resolved, v.Token, val, 1)
					continue
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
			resolved = unescapeDollar(resolved)
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

// resolveDomainVar resolves ${SERVICE_FQDN_*} to the referencing service's
// bare host and ${SERVICE_URL_*} to https://host. The token's own key
// suffix is ignored: both mean "my own public address", a per-service
// fact. With no domain assigned it reports false so a ":-default" applies.
func resolveDomainVar(f *File, svcKey string, v MagicVar) (string, bool) {
	if v.Kind != "FQDN" && v.Kind != "URL" {
		return "", false
	}
	domain := f.Domains[svcKey]
	if domain == "" {
		domain = f.RuntimeDomains[svcKey]
	}
	if domain == "" || strings.Contains(domain, "*") {
		return "", false
	}
	if v.Kind == "URL" {
		return "https://" + domain, true
	}
	return domain, true
}

// resolveCommandEntry resolves every magic var in one Command entry the
// same way an embedded Environment token resolves (FQDN and
// bash-default tokens substitute in place), except a generatable-kind
// token: Command has no secret-storage indirection to land a generated
// value in (see ResolveMagicVars), so it's reported as unresolved
// instead of generated.
func resolveCommandEntry(f *File, svcKey string, index int, entry string) (string, []UnresolvedVar) {
	// "$$" is held as a sentinel while tokens resolve, so "$$SERVICE_X" stays
	// a literal for the container shell and is never read as a magic var.
	entry = strings.ReplaceAll(strings.ReplaceAll(entry, "\x00", ""), "$$", dollarSentinel)
	vars := FindMagicVars(entry)
	if len(vars) == 0 {
		return unescapeDollar(entry), nil
	}

	location := fmt.Sprintf("command[%d]", index)
	resolved := entry
	var unresolved []UnresolvedVar
	for _, v := range vars {
		if val, ok := resolveDomainVar(f, svcKey, v); ok {
			resolved = strings.Replace(resolved, v.Token, val, 1)
			continue
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
	return unescapeDollar(resolved), unresolved
}

// unescapeEntries turns Compose's "$$" escape into a literal "$" in argv
// entries that never receive magic-var values, so secrets stay out of argv.
func unescapeEntries(in []string) []string {
	out := make([]string, len(in))
	for i, e := range in {
		out[i] = unescapeDollar(strings.ReplaceAll(strings.ReplaceAll(e, "\x00", ""), "$$", dollarSentinel))
	}
	return out
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

// resolveScalarField expands ${VAR} and ${VAR:-default} in a single-value key
// such as user:; an unset variable without a default is left for validation.
func resolveScalarField(f *File, svcKey, key, value string, interpolate bool) (string, error) {
	if interpolate {
		v, err := interpolateValue(value, f.InterpVars, func(w string) {
			f.InterpWarnings = append(f.InterpWarnings, fmt.Sprintf("service %q: %s: %s", svcKey, key, w))
		})
		if err != nil {
			return "", fmt.Errorf("service %q: %s: %w", svcKey, key, err)
		}
		value = v
	}
	for _, v := range FindMagicVars(value) {
		if rv, ok := resolveDomainVar(f, svcKey, v); ok {
			value = strings.Replace(value, v.Token, rv, 1)
		} else if v.HasDefault && !v.Generatable {
			value = strings.Replace(value, v.Token, v.Default, 1)
		}
	}
	return unescapeDollar(value), nil
}

func hasDollarEscape(svc Service) bool {
	lists := [][]string{svc.Command, svc.Entrypoint}
	if svc.Healthcheck != nil {
		lists = append(lists, svc.Healthcheck.Test)
	}
	for _, l := range lists {
		for _, e := range l {
			if strings.Contains(e, "$$") {
				return true
			}
		}
	}
	return false
}
