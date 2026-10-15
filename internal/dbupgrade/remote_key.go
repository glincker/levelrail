package dbupgrade

// CatalogPublicKey is the base64 ed25519 public key that published catalogs
// must be signed with. It is empty until the owner sets it (see
// docs/database-version-catalog.md); while empty, published refresh is
// disabled and status says why.
const CatalogPublicKey = ""
