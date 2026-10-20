// Docker over SSH import source (internal/api/app_import_ssh*.go). The SSH
// key only travels in request bodies, never the query cache.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface AppImportSSHLogin {
  host: string
  port?: number
  user: string
  private_key?: string
  passphrase?: string
  password?: string
  host_key_fingerprint?: string
  insecure_host_key?: boolean
  socket_path?: string
}

export type SSHFailureKind =
  | 'auth_failed'
  | 'host_key_mismatch'
  | 'host_key_required'
  | 'socket_permission'
  | 'socket_path'
  | 'socket_forbidden'
  | 'unreachable'
  | 'blocked_address'
  | 'other'

export interface SSHHostKeyInfo {
  type: string
  fingerprint: string
}

export interface SSHOverview {
  docker_version: string
  os?: string
  swarm: boolean
  containers: number
  running: number
  volumes: number
  images: number
}

export interface SSHProbeResult {
  ok: boolean
  target?: string
  kind?: SSHFailureKind
  message?: string
  reads?: string
  overview?: SSHOverview
  host_keys?: SSHHostKeyInfo[]
  observed_host_keys?: SSHHostKeyInfo[]
  warnings?: string[]
}

export interface SSHProbeBody {
  ssh: AppImportSSHLogin
  allow_private?: boolean
  allow_loopback?: boolean
}

export type VolumeCopyState =
  'pending' | 'running' | 'copied' | 'failed' | 'skipped'

export interface VolumeCopyResult {
  source_id: string
  app: string
  volume: string
  container_path: string
  state: VolumeCopyState
  files: number
  bytes: number
  skipped_entries: number
  warnings?: string[]
  error?: string
  fallback?: string
}

export interface VolumeCopy {
  running: boolean
  max_bytes: number
  max_files: number
  results: VolumeCopyResult[]
}

const base = '/api/v1/migration/apps'
const enc = encodeURIComponent

async function send<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `request failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

function post(body: unknown): RequestInit {
  return {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }
}

export function useSSHHostKeys() {
  return useMutation<SSHProbeResult, ApiError, SSHProbeBody>({
    mutationFn: (body) =>
      send<SSHProbeResult>(`${base}/ssh/host-keys`, post(body)),
  })
}

export function useSSHTest() {
  return useMutation<SSHProbeResult, ApiError, SSHProbeBody>({
    mutationFn: (body) => send<SSHProbeResult>(`${base}/ssh/test`, post(body)),
  })
}

const copyKey = (id: string) =>
  ['migration', 'apps', 'session', id, 'volume-copy'] as const

export function useVolumeCopy(id: string, enabled: boolean) {
  return useQuery({
    queryKey: copyKey(id),
    enabled,
    queryFn: () => send<VolumeCopy>(`${base}/sessions/${enc(id)}/volumes/copy`),
    refetchInterval: (q) => (q.state.data?.running ? 2000 : false),
  })
}

export function useStartVolumeCopy() {
  const qc = useQueryClient()
  return useMutation<VolumeCopy, ApiError, { id: string; items?: string[] }>({
    mutationFn: ({ id, items }) =>
      send<VolumeCopy>(
        `${base}/sessions/${enc(id)}/volumes/copy`,
        post({ items }),
      ),
    onSuccess: (v, vars) => {
      qc.setQueryData(copyKey(vars.id), v)
      void qc.invalidateQueries({
        queryKey: ['migration', 'apps', 'session', vars.id],
        exact: true,
      })
    },
  })
}
