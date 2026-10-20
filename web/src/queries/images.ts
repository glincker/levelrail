// Query-key factory and fetcher for the image-tag sub-resource of an
// app: GET /api/v1/apps/{name}/images (internal/api/images.go). Kept in
// its own module rather than folded into queries/deploys.ts, mirroring
// that file's own doc comment reasoning for why deploys.ts isn't folded
// into queries/apps.ts: this is genuinely a different resource with a
// different shape (a list of previously-built tags, not a condition
// list or an AppDetail), even though it nests under the same app name
// and exists purely to make DeployTriggerForm.tsx's manual image-tag
// input faster to fill in.
//
// ImageTag is declared here rather than in web/src/types/deploy.ts:
// this pass is scoped to touching only this new file plus
// DeployTriggerForm.tsx, so the wire type lives next to its one
// consumer instead of growing a shared types file this pass has no
// mandate to touch.

import { queryOptions, useQuery } from '@tanstack/react-query'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

// Wire type for GET /api/v1/apps/{name}/images
// (internal/api/images.go's imageResource). Newest first, matching
// docker.ImageInfo's own ordering (internal/docker/client.go's
// ListImages sorts by CreatedAt descending).
export interface ImageTag {
  tag: string
  created_at: string
}

export const imageKeys = {
  list: (appName: string) => [...appKeys.detail(appName), 'images'] as const,
}

// Fetches every locally-present tag under this app's current image's
// repo. GET /api/v1/apps/{name}/images returns a bare array, possibly
// empty (no ImageLister configured on the control plane, the repo has
// never been built, or the app itself has no image set yet); an empty
// array and a request failure both mean the same thing to this hook's
// one caller: "nothing to suggest."
export async function fetchImageTags(appName: string): Promise<ImageTag[]> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(appName)}/images`)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch image tags failed: ${res.status}`),
    )
  }
  const body = (await res.json()) as ImageTag[] | null
  return body ?? []
}

export function imageTagsQueryOptions(appName: string) {
  return queryOptions({
    queryKey: imageKeys.list(appName),
    queryFn: () => fetchImageTags(appName),
  })
}

// For DeployTriggerForm's "previously built" dropdown: not suspense, no
// retry, and any failure (network error, 404 on an app that doesn't
// exist yet in some edge case, a 5xx) degrades to "there is nothing to
// suggest" rather than blocking the form. The deploy trigger form's core
// function, typing an image tag by hand and submitting it, must always
// keep working even if this query never resolves, the same
// graceful-degradation shape queries/nodes.ts's useNodeListOptional
// already establishes for its own optional picker.
export function useImageTagsOptional(appName: string) {
  return useQuery({ ...imageTagsQueryOptions(appName), retry: false })
}
