package runtimesettings

// Organization profile keys.
const (
	KeyOrgsAvatarMaxBytes     = "orgs.avatar_max_bytes"
	KeyOrgsAvatarMaxDimension = "orgs.avatar_max_dimension"
)

func init() {
	Register(Definition{
		Key: KeyOrgsAvatarMaxBytes, Category: CategorySecurity, Kind: KindInt, Default: "262144",
		Env: "APP_ORGS_AVATAR_MAX_BYTES", Apply: ApplyLive, CheckValue: intRangeCheck(16384, 2097152),
		Title:       "Organization avatar: largest upload",
		Description: "Most bytes an organization avatar upload may have, from 16384 to 2097152. The next upload uses a changed value.",
	})
	Register(Definition{
		Key: KeyOrgsAvatarMaxDimension, Category: CategorySecurity, Kind: KindInt, Default: "1024",
		Env: "APP_ORGS_AVATAR_MAX_DIMENSION", Apply: ApplyLive, CheckValue: intRangeCheck(64, 4096),
		Title:       "Organization avatar: largest side in pixels",
		Description: "Widest or tallest an organization avatar may be, from 64 to 4096 pixels. Checked from the image header before any pixels are decoded.",
	})
}
