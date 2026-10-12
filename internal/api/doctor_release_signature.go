package api

import (
	"fmt"
	"slices"

	"github.com/GLINCKER/levelrail/internal/selfupgrade"
	"github.com/GLINCKER/levelrail/kit/upgrade"
)

const releaseSignatureDocsPath = "/upgrade-safely#signature-pin"

func (rt *Router) serverBinaryName() string {
	if rt.brand == nil || rt.brand.BinaryName == "" {
		return "server"
	}
	return rt.brand.BinaryName
}

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
			Message:  "this install is not signature verified: upgrades still accept releases checked by checksum only. Install cosign and upgrade once to pin signed releases",
			Fix:      "sudo sh install.sh install-cosign && sudo " + rt.serverBinaryName() + " self-upgrade --latest",
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
	Pinned, VerifierOK, ReleaseKnown, Edge bool
	Assets                                 []string
	FixCommand                             string
}

// signaturePinCheck blocks an upgrade that a pinned host would refuse
// anyway, and nudges an unpinned host to verify a signature once.
func signaturePinCheck(in signaturePinInputs) upgrade.Check {
	c := upgrade.Check{Code: "signature_pin", Name: "Release signature pin", Protects: "Refuses a release whose signature was stripped, once this host has verified a signed one."}
	switch {
	case in.Edge:
		c.Status, c.Message = upgrade.StatusSkipped, upgrade.EdgeNoSignatureNote
	case !in.Pinned:
		c.Status = upgrade.StatusWarn
		c.Message = "This install is not signature verified yet, so an upgrade still accepts a release checked by checksum only."
		c.Hint = "Install cosign and upgrade once: every later upgrade then requires a valid signature."
		c.Fix = &upgrade.Fix{Action: upgrade.FixCommand, Command: in.FixCommand}
	case !in.VerifierOK:
		c.Status = upgrade.StatusFail
		c.Message = "This host only accepts signed releases, but cosign is not installed, so the upgrade would be refused."
		c.Hint = "Install cosign on the host, then re-check."
		c.Fix = &upgrade.Fix{Action: upgrade.FixCommand, Command: in.FixCommand}
	case in.ReleaseKnown && !slices.Contains(in.Assets, upgrade.SignatureAsset):
		c.Status = upgrade.StatusFail
		c.Message = "This host only accepts signed releases, and this release publishes no signature."
		c.Hint = "Pick a signed release. Overriding needs --verify off on the host, which is logged."
	default:
		c.Status, c.Message = upgrade.StatusOK, "Pinned to signed releases: an unsigned release is refused."
	}
	return c
}
