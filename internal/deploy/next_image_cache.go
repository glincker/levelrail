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

// nextStandaloneDir is where the standalone server runs; it chdirs to its own
// directory, so the cache resolves under it rather than under the working dir.
const nextStandaloneDir = ".next/standalone"

// nextImageCachePath is the in-container mount path for a source build. The
// standalone server writes under .next/standalone, plain next start under the working dir.
func nextImageCachePath(standalone bool) string {
	if standalone {
		return path.Join(rpbuildkit.WorkingDir, nextStandaloneDir, nextImageCacheRel)
	}
	return path.Join(rpbuildkit.WorkingDir, nextImageCacheRel)
}

// nextImageCacheTarget returns the mount path when the service opted in and the
// source is a Next.js app, else empty.
func nextImageCacheTarget(req Request, prof build.NextProfile, standalone bool) string {
	if !req.Service.Next.UsesPersistentImageCache() || !prof.Detected {
		return ""
	}
	return nextImageCachePath(standalone)
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
