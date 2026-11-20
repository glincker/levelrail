package runtimesettings

import "context"

// KeyPagesImageResizeCacheBytes caps the memory held by resized variants.
const KeyPagesImageResizeCacheBytes = "pages.image_resize_cache_bytes"

// PagesImageResize returns the live resize switch, the three source limits and the cache cap.
func (s *Service) PagesImageResize(ctx context.Context) (on bool, maxWidth, maxSourceBytes, maxPixels, cacheBytes int) {
	return s.Bool(ctx, KeyPagesImageResize), s.Limit(ctx, KeyPagesImageResizeMaxWidth),
		s.Limit(ctx, KeyPagesImageResizeMaxSourceBytes), s.Limit(ctx, KeyPagesImageResizeMaxPixels),
		s.Limit(ctx, KeyPagesImageResizeCacheBytes)
}

// KeyPagesImageResize turns on the on-demand image resize route for Pages sites.
const KeyPagesImageResize = "pages.image_resize"

// KeyPagesImageResizeMaxWidth caps the width a resize request may ask for.
const KeyPagesImageResizeMaxWidth = "pages.image_resize_max_width"

// KeyPagesImageResizeMaxSourceBytes caps the source image size that is decoded.
const KeyPagesImageResizeMaxSourceBytes = "pages.image_resize_max_source_bytes"

// KeyPagesImageResizeMaxPixels caps the decoded source pixel count, which bounds memory.
const KeyPagesImageResizeMaxPixels = "pages.image_resize_max_pixels"

func init() {
	Register(Definition{
		Key: KeyPagesImageResize, Category: CategoryNetworking, Kind: KindBool, Default: "false",
		Env: "APP_PAGES_IMAGE_RESIZE", Apply: ApplyLive,
		Title:       "Pages image resizing",
		Description: "Serve resized JPEG and PNG files from Pages sites when a request adds a w query value. Off by default because decoding images costs memory. The next request uses a changed value.",
	})
	Register(Definition{
		Key: KeyPagesImageResizeMaxWidth, Category: CategoryNetworking, Kind: KindInt, Default: "2048",
		Env: "APP_PAGES_IMAGE_RESIZE_MAX_WIDTH", Apply: ApplyLive, CheckValue: intRangeCheck(16, 16384),
		Title:       "Pages image resize width cap",
		Description: "Largest width in pixels a resize request may ask for. Larger requests are served at this width. The next request uses a changed value.",
	})
	Register(Definition{
		Key: KeyPagesImageResizeMaxSourceBytes, Category: CategoryNetworking, Kind: KindInt, Default: "10485760",
		Env: "APP_PAGES_IMAGE_RESIZE_MAX_SOURCE_BYTES", Apply: ApplyLive, CheckValue: intRangeCheck(1024, 1<<30),
		Title:       "Pages image resize source size cap",
		Description: "Largest source image file in bytes that is decoded for resizing. Bigger files are served unchanged. The next request uses a changed value.",
	})
	Register(Definition{
		Key: KeyPagesImageResizeMaxPixels, Category: CategoryNetworking, Kind: KindInt, Default: "40000000",
		Env: "APP_PAGES_IMAGE_RESIZE_MAX_PIXELS", Apply: ApplyLive, CheckValue: intRangeCheck(10000, 1<<31-1),
		Title:       "Pages image resize pixel cap",
		Description: "Largest source image in pixels that is decoded for resizing, which bounds memory use. Bigger images are served unchanged. The next request uses a changed value.",
	})
	registerLimit(Definition{
		Key: KeyPagesImageResizeCacheBytes, Category: CategoryNetworking, Default: "67108864",
		Env: "APP_PAGES_IMAGE_RESIZE_CACHE_BYTES", Apply: ApplyLive,
		Title:       "Pages image resize cache size",
		Description: "Memory in bytes held for resized image variants, oldest dropped first. The next request uses a changed value.",
	}, 1<<20, 1<<32)
}
