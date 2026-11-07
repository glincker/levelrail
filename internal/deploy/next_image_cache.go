package deploy

import (
	"path"

	rpbuildkit "github.com/railwayapp/railpack/buildkit"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/store"
)

// nextImageCacheRel is where Next.js writes optimised images, relative to the
// directory its server starts in (distDir/cache/images with the default distDir).
const nextImageCacheRel = ".next/cache/images"

// NextImageCacheVolume is the per-app volume name; delete cleanup derives it the same way.
func NextImageCacheVolume(service string) string {
	return volumeName(service, "next-images")
}

// nextImageCachePath is the in-container mount path for a source build. Railpack runs
// next start from its working dir, so the cache lives under that dir, not a guessed /app.
func nextImageCachePath() string {
	return path.Join(rpbuildkit.WorkingDir, nextImageCacheRel)
}

// nextImageCacheTarget returns the mount path when the service opted in and the
// source is a Next.js app, else empty.
func nextImageCacheTarget(req Request, prof build.NextProfile) string {
	if !req.Service.Next.UsesPersistentImageCache() || !prof.Detected {
		return ""
	}
	return nextImageCachePath()
}

// applyNextImageCache adds the cache volume once; a missing path leaves desired untouched.
func applyNextImageCache(desired *store.DesiredService, service, mountPath string) {
	if mountPath == "" {
		return
	}
	name := NextImageCacheVolume(service)
	for _, v := range desired.Volumes {
		if v.Name == name || v.ContainerPath == mountPath {
			return
		}
	}
	desired.Volumes = append(desired.Volumes, store.ServiceVolume{Name: name, ContainerPath: mountPath})
}
