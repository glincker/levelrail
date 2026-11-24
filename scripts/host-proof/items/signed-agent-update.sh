# shellcheck shell=bash
# Proof: an agent update installs only when its manifest signature verifies.
# All checks are TODO for G02 (HOST vps2, owner approval). Dry unless PROOF_LIVE=1.

prove_signed_agent_update() {
	# TODO G02: confirm the signed manifest on the host verifies.
	proof_check "TODO manifest signature verifies" \
		ssh -o BatchMode=yes "$PROOF_HOST" levelrail-agent verify-manifest
	# TODO G02: a tampered manifest must be rejected, not applied.
	proof_check "TODO tampered manifest is rejected" \
		ssh -o BatchMode=yes "$PROOF_HOST" levelrail-agent verify-manifest --tampered
	# TODO G02: an unsigned agent binary must not install.
	proof_check "TODO unsigned binary is refused" \
		ssh -o BatchMode=yes "$PROOF_HOST" levelrail-agent install --unsigned
	# TODO G02: the running agent version matches the signed manifest after update.
	proof_check "TODO installed version matches manifest" \
		ssh -o BatchMode=yes "$PROOF_HOST" levelrail-agent version
}
