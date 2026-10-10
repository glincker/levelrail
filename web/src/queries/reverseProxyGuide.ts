// GET /api/v1/system/reverse-proxy (internal/api/reverse_proxy_guide.go).
import { useMutation } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface ReverseProxyHolder {
  port: number
  container: string
  image: string
  kind?: string
}

export interface ReverseProxyPlan {
  domain: string
  proxy: string
  upstream_url: string
  needs_rebind: boolean
  rebind?: string
  snippet: string
  snippet_path?: string
  steps: string[]
}

export interface ReverseProxyGuide {
  holders: ReverseProxyHolder[]
  detected_proxy?: string
  plan?: ReverseProxyPlan
  check?: { resolves_to_host: boolean; reachable: boolean; detail?: string }
}

export interface ReverseProxyRequest {
  domain: string
  proxy?: string
  verify?: boolean
}

export async function fetchReverseProxyGuide(
  req: Partial<ReverseProxyRequest>,
): Promise<ReverseProxyGuide> {
  const q = new URLSearchParams()
  if (req.domain) q.set('domain', req.domain)
  if (req.proxy) q.set('proxy', req.proxy)
  if (req.verify) q.set('verify', 'true')
  const qs = q.toString()
  const res = await fetch(`/api/v1/system/reverse-proxy${qs ? `?${qs}` : ''}`)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `reverse proxy guide failed: ${res.status}`),
    )
  }
  return (await res.json()) as ReverseProxyGuide
}

export function useReverseProxyGuide() {
  return useMutation<ReverseProxyGuide, ApiError, ReverseProxyRequest>({
    mutationFn: (req) => fetchReverseProxyGuide(req),
  })
}
