package api

const secretRevealDocsPath = "/environment-and-secrets#reveal-policy"

// doctorCheckSecretReveal warns when secret values can be read without a
// step-up, or when the policy value is unrecognized (reveal then stays off).
func doctorCheckSecretReveal() doctorCheckResource {
	const code, name = "secret_reveal_policy", "Secret reveal policy"
	policy, valid := revealPolicy()
	res := doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, DocsPath: secretRevealDocsPath}
	switch {
	case !valid:
		res.Status = doctorStatusWarn
		res.Message = envSecretReveal + " is not step_up, disabled or allow, so secret reveal is off until it is fixed"
	case policy == revealPolicyAllow:
		res.Status = doctorStatusWarn
		res.Message = envSecretReveal + "=allow: anyone holding read:sensitive can read secret values without confirming their password; use step_up outside a single-admin lab"
	case policy == revealPolicyDisabled:
		res.Message = "secret values are write-only (" + envSecretReveal + "=disabled)"
	default:
		res.Message = "revealing a secret value needs a password, authenticator code or passkey confirmation"
	}
	if valid && policy != revealPolicyDisabled && tokenRevealAllowed() {
		res.Message += "; API tokens with secrets:reveal may also reveal (" + envSecretRevealTokens + "=on)"
	}
	return res
}
