package spec

import "fmt"

// Values of next.cache.
const (
	NextCacheNone  = "none"
	NextCacheRedis = "redis"
)

// Next holds Next.js options for a service. Nil or cache none changes nothing.
type Next struct {
	// Cache picks the shared ISR and data cache store: none (per replica files) or redis.
	Cache string `yaml:"cache,omitempty"`
}

// UsesRedisCache reports whether the service opted into the shared Redis cache.
func (n *Next) UsesRedisCache() bool { return n != nil && n.Cache == NextCacheRedis }

func (svc *Service) validateNext(name string) error {
	if svc.Next == nil {
		return nil
	}
	switch svc.Next.Cache {
	case "", NextCacheNone, NextCacheRedis:
		return nil
	}
	return fmt.Errorf("spec: service %q: next.cache %q is not one of %s, %s", name, svc.Next.Cache, NextCacheNone, NextCacheRedis)
}
