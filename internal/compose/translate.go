package compose

import (
	"fmt"

	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ToDesiredServices translates f into one store.DesiredService per
// compose service, named "<appName>-<serviceKey>" and linked to
// appName as their AppID (matching the naming convention
// internal/deploy's own multi-service fan-out uses). Runs Validate
// first, so a caller only needs to call this one function.
func ToDesiredServices(appName string, f *File) ([]store.DesiredService, error) {
	if err := f.Validate(); err != nil {
		return nil, err
	}

	names := sortedServiceNames(f)
	out := make([]store.DesiredService, 0, len(names))
	for _, key := range names {
		svc := f.Services[key]
		if err := spec.ValidateLabels(svc.Labels); err != nil {
			return nil, fmt.Errorf("service %q: %w", key, err)
		}

		d := store.DesiredService{
			Name:   appName + "-" + key,
			AppID:  appName,
			Image:  svc.Image,
			Env:    map[string]string(svc.Environment),
			Labels: svc.Labels,
		}
		for _, p := range svc.Ports {
			d.Port = p.ContainerPort
			break
		}
		if domain := f.Domains[key]; domain != "" {
			d.Domains = []string{domain}
		}
		for _, v := range svc.Volumes {
			d.Volumes = append(d.Volumes, store.ServiceVolume{
				Name:          volumeName(appName, key, v.Name),
				ContainerPath: v.ContainerPath,
			})
		}
		out = append(out, d)
	}
	return out, nil
}

// volumeName gives a compose service's own logical volume name a
// platform-prefixed, globally-unique Docker volume name, mirroring
// internal/deploy's own volumeName convention one level up (app +
// service scoped, since two compose files can each declare a "data"
// volume).
func volumeName(appName, serviceKey, logicalName string) string {
	return "app-" + appName + "-" + serviceKey + "-" + logicalName
}
