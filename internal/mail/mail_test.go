package mail

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

func TestAccountSpecRedactsPassword(t *testing.T) {
	const pw = "p4ss-secret-value"
	a := AccountSpec{Name: "bob", Address: "bob@example.test", Password: pw}
	out := fmt.Sprintf("%v %+v %#v %s", a, a, a, &a)
	var buf bytes.Buffer
	slog.New(slog.NewTextHandler(&buf, nil)).Info("x", "spec", a)
	out += buf.String()
	if strings.Contains(out, pw) || !strings.Contains(out, "bob@example.test") {
		t.Fatalf("bad output: %s", out)
	}
}
