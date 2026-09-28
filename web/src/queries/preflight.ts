// Preflight checks run on demand, so both are mutations rather than cached
// queries: nothing fires until an operator asks.
import { useMutation } from '@tanstack/react-query'
import type { PreflightReport, PreflightRequest } from '../types/preflight'
import { ApiError, readErrorMessage } from '../lib/apiError'

async function postPreflight(
  url: string,
  body: PreflightRequest,
): Promise<PreflightReport> {
  const res = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `preflight failed: ${res.status}`),
    )
  }
  return (await res.json()) as PreflightReport
}

export function usePreflightApp(appName: string) {
  return useMutation({
    mutationFn: (body: PreflightRequest = {}) =>
      postPreflight(
        `/api/v1/apps/${encodeURIComponent(appName)}/preflight`,
        body,
      ),
  })
}

export function usePreflightNewApp() {
  return useMutation({
    mutationFn: (body: PreflightRequest) =>
      postPreflight('/api/v1/preflight', body),
  })
}
