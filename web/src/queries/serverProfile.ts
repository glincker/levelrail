import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type ServerProfileId = 'minimal' | 'standard' | 'suite'

export interface ServerProfileInfo {
  id: ServerProfileId
  mesh: boolean
  plugins: boolean
  experimental_features: number
  retention_days: number
  loopback_agent: boolean
  lazy_loops: boolean
  posture_recommendations: boolean
  sbom_scan: boolean
}

export interface ServerProfileProblem {
  code: string
  message: string
  blocking: boolean
}

// Matches internal/api's serverProfileResource.
export interface ServerProfile {
  active: ServerProfileId
  configured: ServerProfileId
  source: 'env' | 'setting' | 'default'
  env_var: string
  pinned_by_env: boolean
  restart_required: boolean
  recommended?: ServerProfileId
  profiles: ServerProfileInfo[]
  problems: ServerProfileProblem[]
  needs_confirm: boolean
  saved: boolean
  updated_by?: string
  error?: string
}

export const serverProfileKeys = { all: ['server-profile'] as const }

const URL = '/api/v1/settings/server-profile'

async function fetchServerProfile(): Promise<ServerProfile> {
  const res = await fetch(URL)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch server profile failed: ${res.status}`),
    )
  }
  return (await res.json()) as ServerProfile
}

export function serverProfileQueryOptions() {
  return queryOptions({
    queryKey: serverProfileKeys.all,
    queryFn: fetchServerProfile,
    staleTime: 10_000,
  })
}

export function useServerProfile() {
  return useQuery(serverProfileQueryOptions())
}

interface SetServerProfileInput {
  profile: ServerProfileId
  confirm?: boolean
}

// A refused move (409) still carries the guard problems, so it resolves with saved=false.
export interface SetServerProfileResult {
  saved: boolean
  profile: ServerProfile
}

async function setServerProfile(
  input: SetServerProfileInput,
): Promise<SetServerProfileResult> {
  const res = await fetch(URL, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
  if (res.status === 409) {
    const body = (await res.json()) as ServerProfile
    if (Array.isArray(body.problems) && body.problems.length > 0) {
      return { saved: false, profile: body }
    }
    throw new ApiError(
      res.status,
      body.error ?? `set server profile failed: ${res.status}`,
    )
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `set server profile failed: ${res.status}`),
    )
  }
  return { saved: true, profile: (await res.json()) as ServerProfile }
}

export function useSetServerProfile() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: setServerProfile,
    onSuccess: (data) => {
      if (data.saved) {
        queryClient.setQueryData(serverProfileKeys.all, data.profile)
      }
    },
  })
}
