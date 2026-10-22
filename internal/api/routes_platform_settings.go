package api

import "net/http"

// registerSettingsRoutes registers the email, observability, DNS, vault, registries, static sites and firewall routes.
func (rt *Router) registerSettingsRoutes(mux *http.ServeMux) {
	// Email settings: same precedent as ingress settings just above.
	// GET is AbilityRead; PUT is AbilityRoot, real infrastructure config.
	mux.HandleFunc("GET /api/v1/settings/email", rt.requireAbility(AbilityRead, rt.handleGetEmailSettings))
	mux.HandleFunc("PUT /api/v1/settings/email", rt.requireAbility(AbilityRoot, rt.handleUpdateEmailSettings))
	// Root: a test send goes to any address, so it is not open to ordinary write tokens.
	mux.HandleFunc("POST /api/v1/settings/email/test", rt.requireAbility(AbilityRoot, rt.handleTestEmail))
	mux.HandleFunc("GET /api/v1/settings/email/status", rt.requireAbility(AbilityRead, rt.handleEmailStatus))
	mux.HandleFunc("GET /api/v1/settings/email/domain-check", rt.requireAbility(AbilityRoot, rt.handleEmailDomainCheck))
	mux.HandleFunc("GET /api/v1/settings/email/deliveries", rt.requireAbility(AbilityRoot, rt.handleListEmailDeliveries))
	mux.HandleFunc("GET /api/v1/settings/email/deliveries/{id}", rt.requireAbility(AbilityRoot, rt.handleGetEmailDelivery))

	// Observability settings: the external Grafana/dashboard link,
	// same precedent as email settings just above. GET is AbilityRead;
	// PUT is AbilityRoot, matching every other instance-level config row.
	mux.HandleFunc("GET /api/v1/settings/observability", rt.requireAbility(AbilityRead, rt.handleGetObservabilitySettings))
	mux.HandleFunc("PUT /api/v1/settings/observability", rt.requireAbility(AbilityRoot, rt.handleUpdateObservabilitySettings))

	// Cloudflare Tunnel (instance-level, one connection per control
	// plane): GET is AbilityRead; PUT/DELETE are AbilityRoot, matching
	// PUT /api/v1/settings/email's own tier for infrastructure config
	// that changes how this control plane is reachable.
	mux.HandleFunc("GET /api/v1/settings/cloudflare-tunnel", rt.requireAbility(AbilityRead, rt.handleGetCloudflareTunnelSettings))
	mux.HandleFunc("PUT /api/v1/settings/cloudflare-tunnel", rt.requireAbility(AbilityRoot, rt.handleUpdateCloudflareTunnelSettings))
	mux.HandleFunc("DELETE /api/v1/settings/cloudflare-tunnel", rt.requireAbility(AbilityRoot, rt.handleDisconnectCloudflareTunnel))
	mux.HandleFunc("GET /api/v1/settings/cloudflare-dns", rt.requireAbility(AbilityRead, rt.handleGetCloudflareDNSSettings))
	mux.HandleFunc("PUT /api/v1/settings/cloudflare-dns", rt.requireAbility(AbilityRoot, rt.handleUpdateCloudflareDNSSettings))
	mux.HandleFunc("DELETE /api/v1/settings/cloudflare-dns", rt.requireAbility(AbilityRoot, rt.handleDisconnectCloudflareDNS))
	// Route53 DNS-01: a second, independent ACME DNS-01 provider, same
	// tier as cloudflare-dns above.
	mux.HandleFunc("GET /api/v1/settings/route53-dns", rt.requireAbility(AbilityRead, rt.handleGetRoute53DNSSettings))
	mux.HandleFunc("PUT /api/v1/settings/route53-dns", rt.requireAbility(AbilityRoot, rt.handleUpdateRoute53DNSSettings))
	mux.HandleFunc("DELETE /api/v1/settings/route53-dns", rt.requireAbility(AbilityRoot, rt.handleDisconnectRoute53DNS))
	mux.HandleFunc("GET /api/v1/settings/google-cloud-dns", rt.requireAbility(AbilityRead, rt.handleGetGoogleCloudDNSSettings))
	mux.HandleFunc("PUT /api/v1/settings/google-cloud-dns", rt.requireAbility(AbilityRoot, rt.handleUpdateGoogleCloudDNSSettings))
	mux.HandleFunc("DELETE /api/v1/settings/google-cloud-dns", rt.requireAbility(AbilityRoot, rt.handleDisconnectGoogleCloudDNS))
	mux.HandleFunc("POST /api/v1/settings/google-cloud-dns/test", rt.requireAbility(AbilityRoot, rt.handleTestGoogleCloudDNS))

	// External HashiCorp Vault (instance-level, one connection per
	// control plane): same GET AbilityRead / PUT+DELETE AbilityRoot tier
	// as Cloudflare Tunnel above.
	mux.HandleFunc("GET /api/v1/settings/vault", rt.requireAbility(AbilityRead, rt.handleGetVaultSettings))
	mux.HandleFunc("PUT /api/v1/settings/vault", rt.requireAbility(AbilityRoot, rt.handleUpdateVaultSettings))
	mux.HandleFunc("POST /api/v1/settings/vault/rotate-credential", rt.requireAbility(AbilityRoot, rt.handleRotateVaultCredential))
	mux.HandleFunc("DELETE /api/v1/settings/vault", rt.requireAbility(AbilityRoot, rt.handleDisconnectVault))

	// Built-in container registry (instance-level, one registry per
	// control plane): GET is AbilityRead; PUT/DELETE are AbilityRoot,
	// matching PUT /api/v1/settings/cloudflare-tunnel's own tier for
	// infrastructure config that runs a system container and generates
	// credentials.
	mux.HandleFunc("GET /api/v1/settings/registry", rt.requireAbility(AbilityRead, rt.handleGetRegistrySettings))
	mux.HandleFunc("PUT /api/v1/settings/registry", rt.requireAbility(AbilityRoot, rt.handleUpdateRegistrySettings))
	mux.HandleFunc("DELETE /api/v1/settings/registry", rt.requireAbility(AbilityRoot, rt.handleDisableRegistry))

	// Built-in registry catalog (registry_catalog.go): read-only browsing
	// for the app-creation "existing image" step's repository/tag picker.
	// AbilityRead, same tier as the settings GET just above: no secret is
	// ever returned, the resolved password is only used server-side to
	// authenticate the upstream catalog query.
	mux.HandleFunc("GET /api/v1/registry/repositories", rt.requireAbility(AbilityRead, rt.handleListRegistryRepositories))
	mux.HandleFunc("GET /api/v1/registry/tags", rt.requireAbility(AbilityRead, rt.handleListRegistryTags))

	// Public Docker Hub search (dockerhub_search.go): the same picker's
	// third source, for a well-known public image that isn't in the
	// built-in registry or a connected credential. AbilityRead, same tier
	// as the built-in catalog just above: a read of a public,
	// unauthenticated upstream, proxied server-side only to avoid a
	// browser-to-hub.docker.com CORS call.
	mux.HandleFunc("POST /api/v1/git-providers/{provider}/validate", rt.requireAbility(AbilityRoot, rt.handleValidateGitProvider))
	mux.HandleFunc("GET /api/v1/dockerhub/search", rt.requireAbility(AbilityRead, rt.handleDockerHubSearch))
	mux.HandleFunc("GET /api/v1/dockerhub/repositories/{namespace}/{repo}/tags", rt.requireAbility(AbilityRead, rt.handleDockerHubTags))

	// Domains (centralized cross-app list, web/src/routes/domains):
	// every service_domains row, AbilityRead like GET /api/v1/apps,
	// no new ability tier: this is the same data DomainEditor already
	// exposes per-app, aggregated across every app in one read-only call.
	mux.HandleFunc("GET /api/v1/domains", rt.requireAbility(AbilityRead, rt.handleListDomains))

	// Static sites (build.type: static): read-only
	// dashboard visibility for sites served directly by embedded Caddy
	// with no container, closing the gap flagged when static-site
	// support (migration 0015) first landed. AbilityRead, the same
	// passive-visibility boundary as handleListCertificates above: no
	// create/update/delete route exists yet because the backend has no
	// mutation path for a static site beyond internal/deploy.Pipeline's
	// own git-push-triggered write.
	mux.HandleFunc("GET /api/v1/static-sites", rt.requireAbility(AbilityRead, rt.handleListStaticSites))
	mux.HandleFunc("DELETE /api/v1/static-sites/{name}", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleDeleteStaticSite))
	mux.HandleFunc("GET /api/v1/apps/{name}/static-routes", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetStaticRoutes))
	mux.HandleFunc("POST /api/v1/apps/{name}/static-routes/test", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleTestStaticRoutes))
	mux.HandleFunc("POST /api/v1/apps/{name}/static-routes/lint", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleLintStaticRoutes))

	// Backup targets (connected S3-compatible buckets a database backup
	// can be uploaded to). Create and delete need AbilityWriteSensitive:
	// create because its request body carries live bucket credentials,
	// delete because it gates access to the same. List/get are ordinary
	// AbilityRead, the same boundary handleListCertificates/
	// handleListStaticSites already draw between visibility and mutation.
	mux.HandleFunc("GET /api/v1/backup-targets", rt.requireAbility(AbilityRead, rt.handleListBackupTargets))
	mux.HandleFunc("POST /api/v1/backup-targets", rt.requireAbility(AbilityWriteSensitive, rt.handleCreateBackupTarget))
	mux.HandleFunc("GET /api/v1/backup-targets/{id}", rt.requireAbility(AbilityRead, rt.handleGetBackupTarget))
	mux.HandleFunc("PUT /api/v1/backup-targets/{id}", rt.requireAbility(AbilityWriteSensitive, rt.handleUpdateBackupTarget))
	mux.HandleFunc("DELETE /api/v1/backup-targets/{id}", rt.requireAbility(AbilityWriteSensitive, rt.handleDeleteBackupTarget))
	mux.HandleFunc("GET /api/v1/backup-targets/{id}/encryption", rt.requireAbility(AbilityRead, rt.handleGetBackupTargetEncryption))
	mux.HandleFunc("PUT /api/v1/backup-targets/{id}/encryption", rt.requireAbility(AbilityRoot, rt.handleSetBackupTargetEncryption))
	mux.HandleFunc("POST /api/v1/backup-targets/{id}/test", rt.requireAbility(AbilityWriteSensitive, rt.handleTestBackupTarget))

	// Registry credentials (registry_credentials.go): same ability tiers
	// as backup targets just above, same reasoning (POST/PUT/DELETE
	// handle live pull credentials).
	mux.HandleFunc("GET /api/v1/registry-credentials", rt.requireAbility(AbilityRead, rt.handleListRegistryCredentials))
	mux.HandleFunc("POST /api/v1/registry-credentials", rt.requireAbility(AbilityWriteSensitive, rt.handleCreateRegistryCredential))
	mux.HandleFunc("GET /api/v1/registry-credentials/{id}", rt.requireAbility(AbilityRead, rt.handleGetRegistryCredential))
	mux.HandleFunc("PUT /api/v1/registry-credentials/{id}", rt.requireAbility(AbilityWriteSensitive, rt.handleUpdateRegistryCredential))
	mux.HandleFunc("DELETE /api/v1/registry-credentials/{id}", rt.requireAbility(AbilityWriteSensitive, rt.handleDeleteRegistryCredential))
	mux.HandleFunc("POST /api/v1/registry-credentials/{id}/test", rt.requireAbility(AbilityWriteSensitive, rt.handleTestRegistryCredential))
	mux.HandleFunc("GET /api/v1/registry-credentials/{id}/usage", rt.requireAbility(AbilityRead, rt.handleRegistryCredentialUsage))
	mux.HandleFunc("POST /api/v1/registry-credentials/presets/test", rt.requireAbility(AbilityWriteSensitive, rt.handleTestRegistryPreset))
	mux.HandleFunc("POST /api/v1/registry-credentials/{id}/refresh", rt.requireAbility(AbilityWriteSensitive, rt.handleRefreshRegistryCredential))
	mux.HandleFunc("POST /api/v1/registry-credentials/test", rt.requireAbility(AbilityWriteSensitive, rt.handleProbeRegistryCredential))
	mux.HandleFunc("GET /api/v1/registry-vendors", rt.requireAbility(AbilityRead, rt.handleListRegistryVendors))
	// Private package registries for builds: secrets mounted into
	// Dockerfile builds, never returned by any route.
	mux.HandleFunc("GET /api/v1/package-registries", rt.requireAbility(AbilityRead, rt.handleListPackageRegistries))
	mux.HandleFunc("POST /api/v1/package-registries", rt.requireAbility(AbilityWriteSensitive, rt.handleCreatePackageRegistry))
	mux.HandleFunc("DELETE /api/v1/package-registries/{id}", rt.requireAbility(AbilityWriteSensitive, rt.handleDeletePackageRegistry))
	mux.HandleFunc("POST /api/v1/package-registries/{id}/test", rt.requireAbility(AbilityWriteSensitive, rt.handleTestPackageRegistry))

	// Network shares (network_shares.go): an NFS/CIFS export mountable
	// as a Docker local-driver volume. Same ability tiers as registry
	// credentials just above: POST/PUT/DELETE handle a live CIFS
	// password, test is a read-only reachability dial.
	mux.HandleFunc("GET /api/v1/network-shares", rt.requireAbility(AbilityRead, rt.handleListNetworkShares))
	mux.HandleFunc("POST /api/v1/network-shares", rt.requireAbility(AbilityWriteSensitive, rt.handleCreateNetworkShare))
	mux.HandleFunc("GET /api/v1/network-shares/{id}", rt.requireAbility(AbilityRead, rt.handleGetNetworkShare))
	mux.HandleFunc("PUT /api/v1/network-shares/{id}", rt.requireAbility(AbilityWriteSensitive, rt.handleUpdateNetworkShare))
	mux.HandleFunc("DELETE /api/v1/network-shares/{id}", rt.requireAbility(AbilityWriteSensitive, rt.handleDeleteNetworkShare))
	mux.HandleFunc("POST /api/v1/network-shares/{id}/test", rt.requireAbility(AbilityRead, rt.handleTestNetworkShare))

	// Firewall rules (firewall_rules.go): declarative host firewall
	// rules internal/reconcile/firewall converges onto ufw.
	// AbilityWriteSensitive for create/delete, same tier as a backup
	// target: a rule here changes what inbound traffic this host
	// accepts. List is ordinary AbilityRead.
	mux.HandleFunc("GET /api/v1/firewall-rules", rt.requireAbility(AbilityRead, rt.handleListFirewallRules))
	mux.HandleFunc("POST /api/v1/firewall-rules", rt.requireAbility(AbilityWriteSensitive, rt.handleCreateFirewallRule))
	mux.HandleFunc("DELETE /api/v1/firewall-rules/{id}", rt.requireAbility(AbilityWriteSensitive, rt.handleDeleteFirewallRule))
	mux.HandleFunc("GET /api/v1/firewall/host", rt.requireAbility(AbilityRead, rt.handleHostFirewallStatus))
	mux.HandleFunc("POST /api/v1/firewall/host/enable", rt.requireAbility(AbilityRoot, rt.handleEnableHostFirewall))
	mux.HandleFunc("POST /api/v1/firewall/host/disable", rt.requireAbility(AbilityRoot, rt.handleDisableHostFirewall))

	// Exposure audit (exposure.go): read-only list of published container
	// ports and how reachable each is; preview is a pure computation.
	// Restricting and unrestricting change the host's DOCKER-USER chain,
	// so they are AbilityRoot like the ufw switch above.
	mux.HandleFunc("GET /api/v1/firewall/exposure", rt.requireAbility(AbilityRead, rt.handleExposureReport))
	mux.HandleFunc("POST /api/v1/firewall/exposure/preview", rt.requireAbility(AbilityRoot, rt.handleExposurePreview))
	mux.HandleFunc("PUT /api/v1/firewall/exposure/restrictions/{protocol}/{port}", rt.requireAbility(AbilityRoot, rt.handleExposureRestrict))
	mux.HandleFunc("DELETE /api/v1/firewall/exposure/restrictions/{protocol}/{port}", rt.requireAbility(AbilityRoot, rt.handleExposureUnrestrict))

	// Registry credential browsing (registry_catalog.go): repository/tag
	// lookup for a stored external credential, the same generic catalog
	// client GET /api/v1/registry/repositories and /api/v1/registry/tags
	// use above for the built-in registry. AbilityReadSensitive, the same
	// tier GET /api/v1/git-providers and the github-app/gitlab-app/
	// bitbucket-app repo-browsing routes below use: read-only, but only
	// works because a stored credential's secret is resolved server-side.
	mux.HandleFunc("GET /api/v1/registry-credentials/{id}/repositories", rt.requireAbility(AbilityReadSensitive, rt.handleListRegistryCredentialRepositories))
	mux.HandleFunc("GET /api/v1/registry-credentials/{id}/tags", rt.requireAbility(AbilityReadSensitive, rt.handleListRegistryCredentialTags))
}
