package upgrade

import "fmt"

// CodeSchemaCompat is the preflight check that compares the target release's
// schema version with the database's.
const CodeSchemaCompat = "schema_compat"

// FixOpenRollback sends the operator to the rollback section instead of
// offering an upgrade the database cannot accept.
const FixOpenRollback = "open_rollback"

// Check kinds for the schema check.
const (
	KindSchemaOlder      = "schema_older"
	KindSchemaUnverified = "schema_unverified"
)

// DowngradeRefusal is the shared reason text of the pre-click check and the
// upgrade's own downgrade guard, so both say the same thing.
const DowngradeRefusal = "target release is older than the database schema"

// Schema states a caller reports for the target release.
const (
	SchemaKnown       = "known"
	SchemaMissing     = "missing"
	SchemaUnverified  = "unverified"
	SchemaUnavailable = "unavailable"
)

// SchemaInput is the target release's schema, resolved by the caller from the
// signed release manifest. DB and Release are -1 when unknown.
type SchemaInput struct {
	Tag     string
	State   string
	DB      int
	Release int
	// Detail explains an unverified state.
	Detail string
}

// DowngradeMessage is the refusal text for a target that supports a lower
// schema than the database already has.
func DowngradeMessage(dbSchema int, tag string, supports int) string {
	return fmt.Sprintf("%s: database is at schema %d, %s supports %d", DowngradeRefusal, dbSchema, tag, supports)
}

// SchemaBlocked reports whether the schema check is what stops this target.
func SchemaBlocked(checks []Check) bool {
	for _, c := range checks {
		if c.Code == CodeSchemaCompat && c.Status == StatusFail {
			return true
		}
	}
	return false
}

func schemaCheck(in SchemaInput) Check {
	c := Check{
		Code: CodeSchemaCompat, Name: "Compatible with your database",
		Protects: "Keeps you from being offered a release that is older than your database and would refuse to start.",
	}
	switch in.State {
	case SchemaKnown:
		return schemaKnownCheck(c, in)
	case SchemaUnverified:
		c.Status, c.Kind = StatusWarn, KindSchemaUnverified
		c.Message = "the release's schema manifest could not be verified, so compatibility will be checked after download"
		if in.Detail != "" {
			c.Message += " (" + in.Detail + ")"
		}
		c.Fix = &Fix{Action: FixRecheck}
	case SchemaUnavailable:
		c.Status = StatusUnknown
		c.Message = "the release's schema manifest could not be fetched, so compatibility will be checked after download"
		c.Fix = &Fix{Action: FixRecheck}
	default:
		c.Status = StatusUnknown
		c.Message = "this release does not publish its schema version, so compatibility will be checked after download"
	}
	return c
}

func schemaKnownCheck(c Check, in SchemaInput) Check {
	switch {
	case in.DB < 0:
		c.Status, c.Message = StatusUnknown, fmt.Sprintf("%s supports schema %d, your database schema could not be read", in.Tag, in.Release)
	case in.Release < in.DB:
		c.Status, c.Kind = StatusFail, KindSchemaOlder
		c.Message = DowngradeMessage(in.DB, in.Tag, in.Release)
		c.Hint = "This release is older than your database. Stay on your current version, or use Roll back to go to an older release on purpose."
		c.Fix = &Fix{Action: FixOpenRollback}
	case in.Release == in.DB:
		c.Status, c.Message = StatusOK, fmt.Sprintf("%s uses the same database schema (%d)", in.Tag, in.DB)
	default:
		c.Status, c.Message = StatusOK, fmt.Sprintf("%s will migrate your database from schema %d to %d", in.Tag, in.DB, in.Release)
	}
	return c
}
