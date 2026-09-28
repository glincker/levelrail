import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import type {
  NodeProviderRegionResource,
  NodeProviderResource,
  NodeProviderSizeResource,
  NodeProvisionResource,
} from '../types/nodeProvision'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const nodeProviderKeys = {
  all: ['node-providers'] as const,
  list: () => [...nodeProviderKeys.all, 'list'] as const,
  regions: (provider: string) =>
    [...nodeProviderKeys.all, provider, 'regions'] as const,
  sizes: (provider: string, region: string) =>
    [...nodeProviderKeys.all, provider, 'sizes', region] as const,
}

export const nodeProvisionKeys = {
  all: ['node-provisions'] as const,
  list: () => [...nodeProvisionKeys.all, 'list'] as const,
  detail: (id: string) => [...nodeProvisionKeys.all, 'detail', id] as const,
}

async function fetchNodeProviders(): Promise<NodeProviderResource[]> {
  const res = await fetch('/api/v1/node-providers')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch node providers failed: ${res.status}`),
    )
  }
  return (await res.json()) as NodeProviderResource[]
}

export function nodeProviderListQueryOptions() {
  return queryOptions({
    queryKey: nodeProviderKeys.list(),
    queryFn: fetchNodeProviders,
  })
}

export function useNodeProviders() {
  return useQuery(nodeProviderListQueryOptions())
}

// The fields after token are aws-only: hetzner/digitalocean use token
// alone as their single bearer API token, aws uses token as the access
// key id paired with secretAccessKey (or useAmbientCredentials to skip
// both and use this control plane's own AWS identity instead).
interface SetNodeProviderCredentialInput {
  provider: string
  token: string
  secret_access_key?: string
  session_token?: string
  region?: string
  role_arn?: string
  use_ambient_credentials?: boolean
}

async function setNodeProviderCredential(
  input: SetNodeProviderCredentialInput,
): Promise<NodeProviderResource> {
  const res = await fetch('/api/v1/node-providers', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `set node provider credential failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as NodeProviderResource
}

export function useSetNodeProviderCredential() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: setNodeProviderCredential,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: nodeProviderKeys.list() })
    },
  })
}

async function fetchNodeProviderRegions(
  provider: string,
): Promise<NodeProviderRegionResource[]> {
  const res = await fetch(
    `/api/v1/node-providers/${encodeURIComponent(provider)}/regions`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch node provider regions failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as NodeProviderRegionResource[]
}

// enabled gates the query on a provider actually being selected in the
// wizard: there's nothing to fetch before that, and every known provider
// requiring a stored credential means an unconditional fetch would just
// 400 on first render.
export function useNodeProviderRegions(provider: string, enabled: boolean) {
  return useQuery({
    queryKey: nodeProviderKeys.regions(provider),
    queryFn: () => fetchNodeProviderRegions(provider),
    enabled: enabled && provider !== '',
  })
}

async function fetchNodeProviderSizes(
  provider: string,
  region: string,
): Promise<NodeProviderSizeResource[]> {
  const params = region ? `?region=${encodeURIComponent(region)}` : ''
  const res = await fetch(
    `/api/v1/node-providers/${encodeURIComponent(provider)}/sizes${params}`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch node provider sizes failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as NodeProviderSizeResource[]
}

export function useNodeProviderSizes(
  provider: string,
  region: string,
  enabled: boolean,
) {
  return useQuery({
    queryKey: nodeProviderKeys.sizes(provider, region),
    queryFn: () => fetchNodeProviderSizes(provider, region),
    enabled: enabled && provider !== '' && region !== '',
  })
}

export interface CreateNodeProvisionInput {
  provider: string
  region: string
  size: string
  name: string
  role: 'general' | 'build'
  control_plane_addr: string
}

async function createNodeProvision(
  input: CreateNodeProvisionInput,
): Promise<NodeProvisionResource> {
  const res = await fetch('/api/v1/nodes/provision', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `create node provision failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as NodeProvisionResource
}

export function useCreateNodeProvision() {
  return useMutation({ mutationFn: createNodeProvision })
}

async function fetchNodeProvision(id: string): Promise<NodeProvisionResource> {
  const res = await fetch(`/api/v1/node-provisions/${encodeURIComponent(id)}`)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch node provision failed: ${res.status}`),
    )
  }
  return (await res.json()) as NodeProvisionResource
}

// PROVISION_POLL_INTERVAL_MS is the wizard's own progress-view poll
// cadence: fast enough to feel live, without hammering the provider API
// GET /api/v1/node-provisions/{id} recomputes status against on every
// call. Polling stops once the provision reaches a terminal status.
const PROVISION_POLL_INTERVAL_MS = 3_000

// Callers watch the returned status themselves (see AddNodeWizard) and
// invalidate nodeKeys.list() once it turns "ready": refetchInterval must
// stay a pure function of query state, not a place to run side effects.
export function useNodeProvision(id: string | null) {
  return useQuery({
    queryKey: nodeProvisionKeys.detail(id ?? ''),
    queryFn: () => fetchNodeProvision(id ?? ''),
    enabled: id !== null,
    refetchInterval: (query) => {
      const status = query.state.data?.status
      return status === 'ready' || status === 'failed'
        ? false
        : PROVISION_POLL_INTERVAL_MS
    },
  })
}
