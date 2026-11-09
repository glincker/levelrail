package runtimesettings

import (
	"context"
	"strconv"
	"time"
)

// Template import by URL keys.
const (
	KeyTemplatesImportMaxBytes = "templates.import_max_bytes"
	KeyTemplatesImportTimeout  = "templates.import_timeout"

	templatesImportBytesFloor = 4096
	templatesImportBytesCeil  = 2 << 20
	templatesImportTimeFloor  = time.Second
	templatesImportTimeCeil   = time.Minute
)

func init() {
	registerLimit(Definition{
		Key: KeyTemplatesImportMaxBytes, Category: CategoryNetworking, Default: strconv.Itoa(256 << 10), Env: "APP_TEMPLATE_IMPORT_MAX_BYTES", Apply: ApplyLive,
		Title:       "Templates: import size limit",
		Description: "Largest compose file in bytes that a template import from a URL downloads, from 4096 to 2097152. A bigger file is refused. A change applies to the next import.",
	}, templatesImportBytesFloor, templatesImportBytesCeil)
	Register(Definition{
		Key: KeyTemplatesImportTimeout, Category: CategoryNetworking, Kind: KindDuration, Default: "10s", Env: "APP_TEMPLATE_IMPORT_TIMEOUT", Apply: ApplyLive,
		CheckValue:  durationRangeCheck(templatesImportTimeFloor, templatesImportTimeCeil),
		Title:       "Templates: import timeout",
		Description: "How long a template import from a URL may take in total, from 1s to 1m. A change applies to the next import.",
	})
}

// TemplatesImportTimeout returns the saved timeout forced into its range.
func (s *Service) TemplatesImportTimeout(ctx context.Context) time.Duration {
	d := s.Duration(ctx, KeyTemplatesImportTimeout)
	if d <= 0 {
		return 10 * time.Second
	}
	return min(max(d, templatesImportTimeFloor), templatesImportTimeCeil)
}
