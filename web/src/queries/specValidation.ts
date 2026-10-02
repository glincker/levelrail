// Fetcher and hook for live app.yaml validation
// (POST /api/v1/apps/{name}/validate-spec, internal/api/apps_validate_spec.go):
// the same schema and semantic checks internal/spec.Parse itself runs at
// real deploy time, without deploying or saving anything. Callers debounce
// the yaml they pass in (useDebouncedValue), matching queries/logs.ts's
// own debounce-then-query shape (LogSearchPanel.tsx).

import { useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface SpecValidationIssue {
  path: string
  line: number
  message: string
}

export interface SpecValidationResult {
  valid: boolean
  issues: SpecValidationIssue[]
  services?: number
}

export async function validateSpecYaml(
  appName: string,
  yaml: string,
): Promise<SpecValidationResult> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/validate-spec`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ yaml }),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `validate spec failed: ${res.status}`),
    )
  }
  return (await res.json()) as SpecValidationResult
}

export const specValidationKeys = {
  all: ['spec-validation'] as const,
  detail: (appName: string, yaml: string) =>
    [...specValidationKeys.all, appName, yaml] as const,
}

// useSpecValidation expects yaml to already be debounced by the caller:
// it fires on every distinct value it's given, enabled only once there's
// something worth checking.
export function useSpecValidation(appName: string, yaml: string) {
  return useQuery({
    queryKey: specValidationKeys.detail(appName, yaml),
    queryFn: () => validateSpecYaml(appName, yaml),
    enabled: yaml.trim().length > 0,
    staleTime: 0,
    retry: false,
  })
}
