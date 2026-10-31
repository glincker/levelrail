// Approve-at-provider flow. The start route is a later backend slice; the UI
// only reaches this when detection reports domain_connect.flow === true.

import { ApiError, readErrorMessage } from '../lib/apiError'

export const DOMAIN_CONNECT_START_PATH = '/api/v1/domains/domain-connect/start'

/** POSTs the domain, then opens the provider approval page in a new tab. */
export async function startDomainConnectApply(
  domain: string,
  kind: 'mail' | 'app',
): Promise<string> {
  const res = await fetch(DOMAIN_CONNECT_START_PATH, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ domain, kind }),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `domain connect failed: ${res.status}`),
    )
  }
  const body = (await res.json()) as { url?: string }
  const url = typeof body.url === 'string' ? body.url : ''
  if (!/^https:\/\//i.test(url)) {
    throw new ApiError(502, 'domain connect returned no approval link')
  }
  window.open(url, '_blank', 'noopener,noreferrer')
  return url
}
