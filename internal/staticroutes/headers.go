package staticroutes

import (
	"github.com/GLINCKER/levelrail/internal/spec"
)

// ParseHeaders parses Netlify _headers syntax; bad lines are reported, never applied.
func ParseHeaders(src string, l Limits) ([]HeaderBlock, []Problem) {
	return spec.ParseStaticHeaders(src, l)
}

// ValidateHeaderBlock checks a block built from app.yaml and prepares it.
func ValidateHeaderBlock(b *HeaderBlock, l Limits) error { return spec.ValidateStaticHeaderBlock(b, l) }
