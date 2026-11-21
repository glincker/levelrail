package fielddesc

// DNSProvider is the resource name of the per provider DNS credential form.
const DNSProvider = "dns-providers"

func init() {
	when := func(p string) []Condition { return []Condition{{Field: "provider", Equals: []string{p}}} }
	f := func(id, key string, t Type, req bool, show []Condition, flag, doc string) Field {
		p := "integrations:fields.dnsProvider." + id
		return Field{ID: id, Key: key, Type: t, LabelKey: p + ".label", HelpKey: p + ".help", Required: req, Secret: t == TypeSecret, ShowWhen: show, Flag: flag, Doc: doc}
	}
	provider := f("provider", "provider", TypeSelect, true, nil, "", "DNS provider, one of cloudflare, route53 or googlecloud (the CLI picks it by subcommand)")
	provider.Options = []string{"cloudflare", "route53", "googlecloud"}
	provider.Default = "cloudflare"
	provider.FormOnly = true
	key := f("googlecloud.service_account_json", "service_account_json", TypeText, true, when("googlecloud"), "key-file", "service account JSON key, write only (the CLI reads it from a file)")
	key.Secret, key.FlagFile = true, true

	Register(Resource{Name: DNSProvider, Docs: Docs{File: "dns-provider-fields.md", Title: "DNS provider fields", Description: "Every credential field of the Cloudflare, Route53 and Google Cloud DNS providers with its type, requirement, visibility rule and CLI flag."}, Fields: []Field{
		provider,
		f("cloudflare.token", "token", TypeSecret, true, when("cloudflare"), "cf-api-token", "Cloudflare API token with Zone:DNS:Edit scope, write only"),
		f("route53.access_key_id", "access_key_id", TypeSecret, true, when("route53"), "aws-access-key-id", "AWS IAM access key ID, write only"),
		f("route53.secret_access_key", "secret_access_key", TypeSecret, true, when("route53"), "aws-secret-access-key", "AWS IAM secret access key, write only"),
		f("route53.region", "region", TypeString, false, when("route53"), "aws-region", "AWS region, resolved automatically when empty"),
		f("route53.hosted_zone_id", "hosted_zone_id", TypeString, false, when("route53"), "hosted-zone-id", "Route53 hosted zone ID, resolved by matching the domain when empty"),
		key,
	}})
}
