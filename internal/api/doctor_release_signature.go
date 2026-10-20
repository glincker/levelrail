package api

import (
	"fmt"
	"slices"

	"github.com/GLINCKER/levelrail/internal/selfupgrade"
	"github.com/GLINCKER/levelrail/kit/upgrade"
)

const releaseSignatureDocsPath = "/upgrade-safely#signature-pin"

// doctorCheckReleaseSignature warns until a release signature has been
// verified on this host: before that, upgrades accept checksum-only releases.
func (rt *Router) doctorCheckReleaseSignature() doctorCheckResource {
	const code, name = "signature_pin", "Release signature pin"
	if rt.dataDir == "" {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "data directory not configured"}
	}
	pin, ok, err := selfupgrade.ReadPin(rt.dataDir)
	if err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: fmt.Sprintf("could not read the signature pin: %s", err)}
	}
	if !ok {
		return doctorCheckResource{
			Code: code, Name: name, Status: doctorStatusWarn,
			Message:  "this install is not signature verified yet: the next upgrade verifies the release signature, then only signed releases are accepted",
			DocsPath: releaseSignatureDocsPath,
		}
	}
	msg := "pinned to signed releases: unsigned releases are refused"
	if pin.Version != "" {
		msg = fmt.Sprintf("pinned to signed releases since %s: unsigned releases are refused", pin.Version)
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: msg}
}

type signaturePinInputs struct {
	Pinned, ReleaseKnown, Edge bool
	Assets                     []string
	TrustRoot                  func() string
}

// signaturePinCheck blocks an upgrade that a pinned host would refuse
// anyway, and nudges an unpinned host to verify a signature once.
func signaturePinCheck(in signaturePinInputs) upgrade.Check {
	c := upgrade.Check{Code: "signature_pin", Name: "Release signature pin", Protects: "Refuses a release whose signature was stripped, once this host has verified a signed one."}
	switch {
	case in.Edge:
		c.Status, c.Message = upgrade.StatusSkipped, upgrade.EdgeNoSignatureNote
	case !in.Pinned:
		c.Status = upgrade.StatusOK
		c.Message = "The next upgrade verifies the release signature, then this host only accepts signed releases."
	case in.ReleaseKnown && !slices.Contains(in.Assets, upgrade.SignatureAsset):
		c.Status = upgrade.StatusFail
		c.Message = "This host only accepts signed releases, and this release publishes no signature."
		c.Hint = "Pick a signed release. Overriding needs --verify off on the host, which is logged."
	case in.TrustRoot != nil && in.TrustRoot() == upgrade.TrustRootUnavailable:
		c.Status = upgrade.StatusFail
		c.Message = "This host only accepts signed releases, and the Sigstore trust data needed to check one has not been downloaded yet."
		c.Hint = "This server retries on its own. Re-check once it can reach Sigstore."
		c.Fix = &upgrade.Fix{Action: upgrade.FixRecheck}
	default:
		c.Status, c.Message = upgrade.StatusOK, "Pinned to signed releases: an unsigned release is refused."
	}
	return c
}
