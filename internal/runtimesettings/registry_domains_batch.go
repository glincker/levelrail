package runtimesettings

// KeyDomainsBatchMaxItems caps the items of one POST /api/v1/domains/batch.
const KeyDomainsBatchMaxItems = "domains.batch_max_items"

func init() {
	Register(Definition{
		Key: KeyDomainsBatchMaxItems, Category: CategoryNetworking, Kind: KindInt, Default: "50",
		Env: "APP_DOMAINS_BATCH_MAX_ITEMS", Apply: ApplyLive, CheckValue: intRangeCheck(1, 500),
		Title:       "Domains: bulk action item limit",
		Description: "Most domains one bulk park, move, attach or delete request may carry, from 1 to 500. Larger requests are rejected before any item runs. The next request uses a changed value.",
	})
}
