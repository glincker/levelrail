// Wire type for GET /api/v1/brand (internal/api/brand.go's handleBrand,
// which serves internal/brand.Brand as-is). Field names mirror the Go
// struct's exported field names directly (PascalCase, no JSON struct
// tags on Brand to remap them), the same "match the wire shape exactly"
// call types/appDetail.ts already made for appResource.
//
// Rebrandability rule: no product name string may be hardcoded anywhere
// under /web. This type exists so every consumer reads the name from
// here instead, at runtime, via BrandProvider (components/BrandProvider.tsx).
export interface Brand {
  Name: string
  ShortName: string
  BinaryName: string
  Domain: string
  SupportURL: string
  // Optional: existing mock Brand objects across web/src/**/*.test.tsx predate
  // this field and don't set it, so it stays optional rather than forcing a
  // sweep of every test fixture for a field most of them never read.
  SupportEmail?: string
  PrimaryColor: string
  // Optional for the same reason as SupportEmail: existing mock Brand
  // objects predate this field.
  PrimaryColorDark?: string
  LogoSVG: string
  DocsURL: string
  DiscussionsURL: string
  RepoURL: string
}
