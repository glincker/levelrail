package runtimesettings

// KeyPagesPreviewMaxPerSite caps preview sites kept per Pages site; 0 keeps them all.
const KeyPagesPreviewMaxPerSite = "pages.preview_max_per_site"

func init() {
	registerLimit(Definition{
		Key: KeyPagesPreviewMaxPerSite, Category: CategoryDeploys, Default: "20", Env: "APP_PAGES_PREVIEW_MAX_PER_SITE", Apply: ApplyLive,
		Title:       "Pages: previews kept per site",
		Description: "The most branch previews one Pages site keeps, from 0 to 500. When a new preview is published, the least recently updated ones beyond the limit are removed. 0 keeps every preview.",
	}, 0, 500)
}
