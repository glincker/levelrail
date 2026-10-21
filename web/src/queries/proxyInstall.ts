// POST /api/v1/system/proxy-integration/install: without confirm it returns
// the exact plan (dry run), with confirm it starts the proxy container.
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { proxyIntegrationKeys, type ProxyIntegration } from './proxyIntegration'

export interface ProxyInstallFile {
  path: string
  content: string
}

export interface ProxyInstallMount {
  host: string
  container: string
  read_only: boolean
}

export interface ProxyInstallPlan {
  provider: string
  container: string
  image: string
  ports: number[]
  mounts: ProxyInstallMount[]
  files: ProxyInstallFile[]
  restart: string
  route_dir: string
  cert_resolver: string
  upstream_host: string
}

export interface ProxyInstallResult {
  dry_run: boolean
  plan: ProxyInstallPlan
  outcome?: 'fixed' | 'still_failing' | 'cannot_fix_here' | 'skipped'
  detail?: string
  integration?: ProxyIntegration
}

export interface ProxyInstallRequest {
  provider: 'traefik'
  confirm: boolean
}

export function useInstallProxy() {
  const queryClient = useQueryClient()
  return useMutation<ProxyInstallResult, ApiError, ProxyInstallRequest>({
    mutationFn: async (req) => {
      const res = await fetch('/api/v1/system/proxy-integration/install', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(req),
      })
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `proxy install failed: ${res.status}`),
        )
      }
      return (await res.json()) as ProxyInstallResult
    },
    onSettled: (_data, _err, req) => {
      if (req.confirm) {
        void queryClient.invalidateQueries({
          queryKey: proxyIntegrationKeys.all,
        })
      }
    },
  })
}
