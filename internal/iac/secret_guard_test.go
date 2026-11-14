package iac

import (
	"errors"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/compose"
)

func TestSecretGuardComposeAttachment(t *testing.T) {
	const value = "hunter2-plaintext-value"
	cases := map[string]string{
		"map form":  "services:\n  db:\n    image: postgres\n    environment:\n      DB_PASSWORD: " + value + "\n",
		"list form": "services:\n  db:\n    image: postgres\n    environment:\n      - DB_PASSWORD=" + value + "\n",
		"label":     "services:\n  db:\n    image: postgres\n    labels:\n      api_secret: " + value + "\n",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			err := compose.ScanSecretLiterals([]byte(src), "compose/db.yaml", compose.StrictLimits{}, LooksSecret)
			var se compose.StrictErrors
			if !errors.As(err, &se) || se[0].Code != compose.CodeStrictSecretLiteral || se[0].Line != 5 {
				t.Fatalf("got %v", err)
			}
			if strings.Contains(err.Error(), value) || !strings.Contains(err.Error(), "compose/db.yaml") {
				t.Fatalf("message must name the file and never the value: %v", err)
			}
		})
	}
	ok := "services:\n  db:\n    image: postgres\n    environment:\n      DB_PASSWORD: ${DB_PASSWORD}\n      DEBUG: \"false\"\n"
	if err := compose.ScanSecretLiterals([]byte(ok), "compose/db.yaml", compose.StrictLimits{}, LooksSecret); err != nil {
		t.Fatalf("interpolated value refused: %v", err)
	}
}
