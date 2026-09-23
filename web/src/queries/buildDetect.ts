// Fetcher for POST /api/v1/build/detect
// (internal/api/build_detect.go's handleDetectFramework): a fast,
// build-free pre-flight check of what Railpack would detect for a git
// source, for CreateAppFromGitFields.tsx to show "Detected: Next.js"
// before the operator commits to a build type. Not a TanStack Query
// `useQuery` hook, unlike queries/gitBranches.ts's useGitBranches: the
// wizard needs to trigger this once repo+branch settle and read the
// result imperatively into local state (so it can pre-select, but not
// lock, the build-type tabs), the same "fire on demand, not on mount"
// shape a mutation gives more naturally than a query would here.

import { useMutation } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface DetectFrameworkInput {
  repoUrl: string
  ref?: string
}

// DetectFrameworkResult mirrors internal/api/build_detect.go's
// detectFrameworkResponse. detected is false whenever frameworkName is
// empty: a repo that can't be cloned or matches no supported provider,
// never surfaced as an error (see that handler's own doc comment), so
// the wizard's fallback is always "keep the manual picker," not a
// scary failure state.
export interface DetectFrameworkResult {
  provider?: string
  frameworkName?: string
  detected: boolean
}

interface RawDetectFrameworkResponse {
  provider?: string
  framework_name?: string
  detected: boolean
}

export async function detectFramework(
  input: DetectFrameworkInput,
): Promise<DetectFrameworkResult> {
  const res = await fetch('/api/v1/build/detect', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      repo_url: input.repoUrl,
      ...(input.ref ? { ref: input.ref } : {}),
    }),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `detect framework failed: ${res.status}`),
    )
  }
  const body = (await res.json()) as RawDetectFrameworkResponse
  return {
    provider: body.provider,
    frameworkName: body.framework_name,
    detected: body.detected,
  }
}

// useDetectFramework is a mutation, not a query: GitBuildSourceFields
// calls .mutate() once a repo URL and branch are both known, the same
// on-demand trigger useGitBranches' own "Load branches" button uses,
// rather than firing automatically on every keystroke.
export function useDetectFramework() {
  return useMutation({
    mutationFn: detectFramework,
  })
}
