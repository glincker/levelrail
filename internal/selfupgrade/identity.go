package selfupgrade

import "regexp"

// CosignOIDCIssuer is the issuer of the keyless certificates release.yml signs with.
const CosignOIDCIssuer = "https://token.actions.githubusercontent.com"

// ReleaseIdentityRegexp matches the release workflow of repo ("owner/name"),
// the only signer identity a release signature may carry.
func ReleaseIdentityRegexp(repo string) string {
	return "(?i)https://github.com/" + regexp.QuoteMeta(repo) + `/\.github/workflows/release\.yml@.*`
}
