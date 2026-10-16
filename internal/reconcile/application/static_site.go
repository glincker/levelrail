package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/store"
)

// ReasonStaticSitePublished is the Ready reason of an app whose content is
// served straight from the embedded file server, with no container.
const ReasonStaticSitePublished = "StaticSitePublished"

// StaticSiteLookup is the optional store surface that tells a placeholder-image
// app apart from a published static site. *store.DB satisfies it.
type StaticSiteLookup interface {
	GetStaticSite(ctx context.Context, name string) (*store.StaticSite, error)
}

// staticSitePublished reports whether this service has a published static
// site. A store without the lookup (test fakes) reports false.
func (c *Controller) staticSitePublished(ctx context.Context) (bool, error) {
	l, ok := c.store.(StaticSiteLookup)
	if !ok {
		return false, nil
	}
	_, err := l.GetStaticSite(ctx, c.serviceName)
	if errors.Is(err, store.ErrServiceNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("lookup: %w", err)
	}
	return true, nil
}
