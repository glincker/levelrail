package api

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var sensitiveJSONName = regexp.MustCompile(`(?i)(secret|token|password)`)

// harmlessSensitiveName matches keys that mention a credential word but carry a flag, count, id or name,
// never the value itself.
var harmlessSensitiveName = regexp.MustCompile(`(?i)^(has_.*|.*_set|.*_count|.*_id|.*_ids|.*_keys|.*_keys_.*|.*_names|.*_held|.*_configured|.*_write_only|.*_usage.*|.*budget.*|.*_tokens|token_reveal|share_.*|copy_.*|allow_.*|environment_secret_env_keys|secret_keys_.*|secrets_.*)$`)

// responseStructName limits the scan to structs sent to clients, since request bodies legitimately carry credentials in.
var responseStructName = regexp.MustCompile(`(Resource|Response)$`)

// sensitiveFieldAllowlist names every API struct field whose JSON key looks like a credential but is
// deliberately serialized. Add an entry only with the reason it is safe (input-only request body, show-once
// value at creation, a name or flag rather than a value).
var sensitiveFieldAllowlist = map[string]string{ //nolint:gosec // field names and reasons, not credentials
	"appResource.SecretEnv":                             "names of secret-backed vars, never values",
	"appResource.Secrets":                               "write-only on create, toAppResource never sets it",
	"branchEnvOverrideResource.Secret":                  "bool flag",
	"environmentEnvEntryResource.Secret":                "bool flag",
	"sharedEnvVarResource.Secret":                       "bool flag",
	"prPreviewEnvResource.Secret":                       "bool flag",
	"environmentCloneResultResource.CopiedSecretValues": "bool flag",
	"gitDisconnectResource.TokenRevoked":                "bool flag, true only when the provider revocation call succeeded",
	"securitySessionsResponse.Tokens":                   "token metadata list (tokenResource carries no secret)",
	"myCredentialsResponse.Tokens":                      "token metadata list (tokenResource carries no secret)",
	"suggestionsResponse.Tokens":                        "token ids, names and ability lists only, no token value or hash",
	"userResource.MustChangePassword":                   "bool flag",
	"ciExchangeResponse.Token":                          "the minted credential, returned to its requester by design",
	"cliPairResponse.Token":                             "minted credential returned once to the CLI that redeemed a single-use pairing code",
	"createNodeJoinTokenResponse.Token":                 "show-once join token at creation",
	"createTokenResponse.Token":                         "show-once API token at creation",
	"reenrollTokenResponse.Token":                       "show-once re-enroll token",
	"mintSessionLinkResponse.Token":                     "show-once sign-in link token for the requester",
	"mobilePairResponse.Token":                          "show-once mobile pairing token",
	"imageUpdateWebhookResource.Token":                  "show-once on rotate",
	"pipelineWebhookResource.Token":                     "show-once on mint or rotate",
	"pipelineWebhookResource.Secret":                    "show-once on mint or rotate",
	"teamInviteResource.Token":                          "show-once on invite creation",
	"eventWebhookResource.SigningSecret":                "show-once on create and rotate",
	"gitSourceResource.WebhookSecret":                   "show-once on create and rotate, never on GET",
	"registrySettingsResource.Password":                 "show-once when a fresh credential is generated",
	"databaseCredentialResource.Password":               "explicit credential reveal endpoint for the database owner",
	"externalDatabasePasswordResponse.Password":         "explicit credential reveal endpoint",
	"buildHookResource.Token":                           "returned once on create and rotate, the only time the owner can copy the hook URL",
	"emailSettingsResource.SMTPPassword":                "shared with the PUT body, GET only fills the *_set flag",
	"emailSettingsResource.SESSecretAccessKey":          "shared with the PUT body, GET only fills the *_set flag",
	"emailSettingsResource.SESSMTPPassword":             "shared with the PUT body, GET only fills the *_set flag",
	"emailSettingsResource.PostmarkServerToken":         "shared with the PUT body, GET only fills the *_set flag",
	"loginResponse.MFAToken":                            "short-lived pending-MFA handle, not a session credential",
	"loginApprovalPollResponse.MFAToken":                "short-lived pending-MFA handle, not a session credential",
	"twoFactorSetupResponse.Secret":                     "TOTP seed shown once to the enrolling user",
	"demoInfoResponse.Password":                         "public demo instance credentials, shown on purpose",
}

// TestAPIStructsDoNotSerializeSecretLookingFields fails when a JSON-tagged struct in this package gains a
// secret, token or password field that is not allowlisted with a reason, so a new response cannot leak one by accident.
func TestAPIStructsDoNotSerializeSecretLookingFields(t *testing.T) {
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var files []*ast.File
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, e.Name(), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
	}
	seen := map[string]bool{}
	var offenders []string
	for _, f := range files {
		{
			ast.Inspect(f, func(n ast.Node) bool {
				ts, ok := n.(*ast.TypeSpec)
				if !ok {
					return true
				}
				st, ok := ts.Type.(*ast.StructType)
				if !ok || !responseStructName.MatchString(ts.Name.Name) {
					return true
				}
				for _, fl := range st.Fields.List {
					key, ok := jsonKey(fl)
					if !ok || !sensitiveJSONName.MatchString(key) || harmlessSensitiveName.MatchString(key) || len(fl.Names) == 0 {
						continue
					}
					id := ts.Name.Name + "." + fl.Names[0].Name
					seen[id] = true
					if _, allowed := sensitiveFieldAllowlist[id]; !allowed {
						offenders = append(offenders, id+` (json:"`+key+`")`)
					}
				}
				return true
			})
		}
	}
	sort.Strings(offenders)
	for _, o := range offenders {
		t.Errorf("%s serializes a secret-looking field: remove it or allowlist it in sensitiveFieldAllowlist with a reason", o)
	}
	for id, reason := range sensitiveFieldAllowlist {
		if strings.TrimSpace(reason) == "" {
			t.Errorf("allowlist entry %s has no reason", id)
		}
		if !seen[id] {
			t.Errorf("allowlist entry %s matches no struct field, remove it", id)
		}
	}
}

func jsonKey(f *ast.Field) (string, bool) {
	if f.Tag == nil {
		return "", false
	}
	tag := strings.Trim(f.Tag.Value, "`")
	_, rest, ok := strings.Cut(tag, `json:"`)
	if !ok {
		return "", false
	}
	val, _, _ := strings.Cut(rest, `"`)
	name, _, _ := strings.Cut(val, ",")
	if name == "" || name == "-" {
		return "", false
	}
	return name, true
}
