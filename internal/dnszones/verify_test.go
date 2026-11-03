package dnszones

import (
	"context"
	"fmt"
	"testing"
)

type codedErr struct{ code string }

func (e codedErr) Error() string     { return "aws: " + e.code }
func (e codedErr) ErrorCode() string { return e.code }

type listFake struct {
	Provider
	err error
}

func (l listFake) Name() string                              { return ProviderRoute53 }
func (l listFake) ListZones(context.Context) ([]Zone, error) { return nil, l.err }

func TestVerifyAccessRoute53Codes(t *testing.T) {
	cases := map[string]string{
		"AccessDenied":         AccessDenied,
		"InvalidClientTokenId": AccessInvalid,
		"Throttling":           AccessError,
	}
	for code, want := range cases {
		rep := VerifyAccess(t.Context(), listFake{err: fmt.Errorf("route53: list hosted zones: %w", codedErr{code})})
		if rep.Outcome != want {
			t.Errorf("%s: outcome %s, want %s", code, rep.Outcome, want)
		}
		if want == AccessDenied && (len(rep.Missing) != 1 || rep.Missing[0] != "route53:ListHostedZones") {
			t.Errorf("missing = %v", rep.Missing)
		}
	}
}
