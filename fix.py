import re

with open("internal/compose/resolve_test.go", "r") as f:
    content = f.read()

# Instead of separate functions, let's append the test cases inside existing functions to avoid duplication warnings from Sonar.

# 1. TestResolveMagicVars_CommandFQDNMissing -> append to TestResolveMagicVars_Command
content = content.replace('		wantUnresolved: []UnresolvedVar{{Service: "app", EnvKey: "command[2]", Token: "$SERVICE_PASSWORD_API"}},\n		},',
'		wantUnresolved: []UnresolvedVar{{Service: "app", EnvKey: "command[2]", Token: "$SERVICE_PASSWORD_API"}},\n		},\n		{\n			name: "Command FQDN missing resolves to unresolved",\n			yaml: `\nservices:\n  app:\n    image: app:latest\n    command: ["sh", "-c", "echo ${SERVICE_FQDN_APP}"]\n`,\n			wantCommand: []string{"sh", "-c", "echo ${SERVICE_FQDN_APP}"},\n			wantUnresolved: []UnresolvedVar{{Service: "app", EnvKey: "command[2]", Token: "${SERVICE_FQDN_APP}"}},\n		},')

# 2. TestResolveMagicVars_PersistErrorPropagates and TestResolveMagicVars_GenerateErrorPropagatesForInvalidKind -> append to TestResolveMagicVars_GenerateErrorPropagates
generate_err_test = """
	_, _, err = ResolveMagicVars(f, func(_, _ string, _ int) (string, error) {
		return "secret", nil
	}, func(svcKey, envKey, value string) error {
		return fmt.Errorf("persist failed")
	})
	if err == nil {
		t.Fatal("ResolveMagicVars() error = nil, want an error propagated from persist")
	}

	f2, _ := Parse([]byte(`
services:
  app:
    image: app:latest
    environment:
      SECRET: $SERVICE_INVALIDKIND_X
`))
	_, _, err = ResolveMagicVars(f2, func(kind, _ string, _ int) (string, error) {
		return GenerateValue(kind, 0)
	}, failPersist(t))
	if err != nil {
		t.Fatalf("ResolveMagicVars() error = %v, want no error for invalid kind", err)
	}
"""
content = content.replace('		t.Fatal("ResolveMagicVars() error = nil, want an error propagated from generate")\n	}', '		t.Fatal("ResolveMagicVars() error = nil, want an error propagated from generate")\n	}\n' + generate_err_test, 1)

# 3. TestResolveMagicVars_EmbeddedPersistErrorPropagates and TestResolveMagicVars_EmbeddedGenerateErrorPropagatesForInvalidKind -> append to TestResolveMagicVars_EmbeddedGenerateErrorPropagates
embedded_err_test = """
	_, _, err = ResolveMagicVars(f, func(_, _ string, _ int) (string, error) {
		return "secret", nil
	}, func(svcKey, envKey, value string) error {
		return fmt.Errorf("persist failed")
	})
	if err == nil {
		t.Fatal("ResolveMagicVars() error = nil, want an error propagated from persist")
	}

	f2, _ := Parse([]byte(`
services:
  app:
    image: app:latest
    environment:
      URL: "postgres://user:$SERVICE_INVALIDKIND_X@db/app"
`))
	_, unresolved, err := ResolveMagicVars(f2, func(kind, _ string, _ int) (string, error) {
		return GenerateValue(kind, 0)
	}, failPersist(t))
	if err != nil {
		t.Fatalf("ResolveMagicVars() error = %v, want no error for invalid kind", err)
	}
	if len(unresolved) != 1 {
		t.Fatalf("unresolved = %+v, want exactly one entry", unresolved)
	}
"""
content = content.replace('		t.Fatal("ResolveMagicVars() error = nil, want an error propagated from generate")\n	}\n}\n\nfunc TestResolveMagicVars_PersistErrorPropagates(t *testing.T) {', '		t.Fatal("ResolveMagicVars() error = nil, want an error propagated from generate")\n	}\n' + embedded_err_test + '}\n\nfunc TestResolveMagicVars_PersistErrorPropagates(t *testing.T) {')

# 4. Remove the original duplicated tests
patterns_to_remove = [
    r'func TestResolveMagicVars_PersistErrorPropagates\(t \*testing\.T\) \{.*?\n\}\n\n',
    r'func TestResolveMagicVars_EmbeddedPersistErrorPropagates\(t \*testing\.T\) \{.*?\n\}\n\n',
    r'func TestResolveMagicVars_CommandFQDNMissing\(t \*testing\.T\) \{.*?\n\}\n\n',
    r'func TestResolveMagicVars_SingleVarFQDNMissing\(t \*testing\.T\) \{.*?\n\}\n\n',
    r'func TestResolveMagicVars_GenerateErrorPropagatesForInvalidKind\(t \*testing\.T\) \{.*?\n\}\n\n',
    r'func TestResolveMagicVars_EmbeddedGenerateErrorPropagatesForInvalidKind\(t \*testing\.T\) \{.*?\n\}\n\n'
]

# We need a proper regex approach since these are multiline
for p in patterns_to_remove:
    content = re.sub(p, '', content, flags=re.DOTALL)

# TestResolveMagicVars_SingleVarFQDNMissing is missing a replacement, we'll just add it to FQDNIsUnresolved
content = content.replace('func TestResolveMagicVars_FQDNIsUnresolved(t *testing.T) {\n	_, _, unresolved := mustResolve(t, `\nservices:\n  app:\n    image: app:latest\n    environment:\n      API_KEY: ${SERVICE_FQDN_ACTIVEPIECES}\n`)\n	if len(unresolved) != 1 {\n		t.Fatalf("unresolved = %+v, want exactly one entry", unresolved)\n	}\n}',
'func TestResolveMagicVars_FQDNIsUnresolved(t *testing.T) {\n	_, _, unresolved := mustResolve(t, `\nservices:\n  app:\n    image: app:latest\n    environment:\n      API_KEY: ${SERVICE_FQDN_ACTIVEPIECES}\n`)\n	if len(unresolved) != 1 {\n		t.Fatalf("unresolved = %+v, want exactly one entry", unresolved)\n	}\n\n	_, _, unresolved2 := mustResolve(t, `\nservices:\n  app:\n    image: app:latest\n    environment:\n      FRONTEND_URL: ${SERVICE_FQDN_APP}\n`)\n	if len(unresolved2) != 1 {\n		t.Fatalf("unresolved = %+v, want exactly one entry", unresolved2)\n	}\n	if unresolved2[0].Token != "${SERVICE_FQDN_APP}" {\n		t.Errorf("unresolved[0].Token = %q, want ${SERVICE_FQDN_APP}", unresolved2[0].Token)\n	}\n}')


with open("internal/compose/resolve_test.go", "w") as f:
    f.write(content)
