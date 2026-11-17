package compose

import "testing"

func TestResolveMagicVars_DollarEscapeInArgv(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"var", `exec x "$$VAR"`, `exec x "$VAR"`},
		{"double", "a$$$$b", "a$$b"},
		{"brace", "$${VAR}", "${VAR}"},
		{"escaped magic stays literal", "$$SERVICE_PASSWORD_X", "$SERVICE_PASSWORD_X"},
		{"escaped brace magic stays literal", "$${SERVICE_PASSWORD_X}", "${SERVICE_PASSWORD_X}"},
		{"no dollar", "plain text", "plain text"},
		{"single dollar unknown", "echo $HOME", "echo $HOME"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := "services:\n  a:\n    image: x:1\n    entrypoint: [\"sh\", \"-c\", '" + tc.in + "']\n" +
				"    command: [\"sh\", \"-c\", '" + tc.in + "']\n" +
				"    healthcheck:\n      test: [\"CMD-SHELL\", '" + tc.in + "']\n"
			f, err := Parse([]byte(body))
			if err != nil {
				t.Fatal(err)
			}
			gen := func(string, string, int) (string, error) { return "SECRET", nil }
			persist := func(string, string, string) error { return nil }
			_, unresolved, err := ResolveMagicVars(f, gen, persist)
			if err != nil || len(unresolved) != 0 {
				t.Fatalf("err=%v unresolved=%v", err, unresolved)
			}
			svc := f.Services["a"]
			got := []string{svc.Entrypoint[2], svc.Command[2], svc.Healthcheck.Test[1]}
			for i, g := range got {
				if g != tc.want {
					t.Errorf("field %d = %q, want %q", i, g, tc.want)
				}
			}
		})
	}
}

func TestResolveMagicVars_UnescapedMagicInCommandStillUnresolved(t *testing.T) {
	body := "services:\n  a:\n    image: x:1\n    command: [\"sh\", \"-c\", \"echo $SERVICE_PASSWORD_X\"]\n"
	f, err := Parse([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	gen := func(string, string, int) (string, error) { return "SECRET", nil }
	_, unresolved, err := ResolveMagicVars(f, gen, func(string, string, string) error { return nil })
	if err != nil || len(unresolved) != 1 {
		t.Fatalf("err=%v unresolved=%v, want secrets in argv kept unresolved", err, unresolved)
	}
}
