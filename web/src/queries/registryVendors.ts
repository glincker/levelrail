// Fetchers for container registry vendor presets, candidate credential
// tests, credential usage and private package registries.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import type {
  ConnectionTestResult,
  CreatePackageRegistryRequest,
  PackageRegistry,
  RegistryCredentialUsage,
  RegistryVendor,
} from '../types/registryVendor'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const registryVendorKeys = {
  vendors: ['registry-vendors'] as const,
  usage: (id: string) => ['registry-credentials', 'usage', id] as const,
  packageRegistries: ['package-registries'] as const,
}

async function getJSON<T>(url: string, what: string): Promise<T> {
  const res = await fetch(url)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

async function sendJSON<T>(
  url: string,
  method: 'POST' | 'DELETE',
  what: string,
  body?: unknown,
): Promise<T> {
  const res = await fetch(url, {
    method,
    headers:
      body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return res.status === 204 ? (undefined as T) : ((await res.json()) as T)
}

export function useRegistryVendors() {
  return useQuery(
    queryOptions({
      queryKey: registryVendorKeys.vendors,
      queryFn: () =>
        getJSON<RegistryVendor[]>('/api/v1/registry-vendors', 'list vendors'),
      staleTime: 5 * 60_000,
    }),
  )
}

export function useRegistryCredentialUsage(id: string) {
  return useQuery(
    queryOptions({
      queryKey: registryVendorKeys.usage(id),
      queryFn: () =>
        getJSON<RegistryCredentialUsage>(
          `/api/v1/registry-credentials/${encodeURIComponent(id)}/usage`,
          'registry credential usage',
        ),
    }),
  )
}

interface ProbeInput {
  registry_host: string
  username: string
  password: string
}

// Authenticates a candidate against its registry and stores nothing.
export function useProbeRegistryCredential() {
  return useMutation({
    mutationFn: (input: ProbeInput) =>
      sendJSON<ConnectionTestResult>(
        '/api/v1/registry-credentials/test',
        'POST',
        'test registry credential',
        input,
      ),
  })
}

export function usePackageRegistries() {
  return useQuery(
    queryOptions({
      queryKey: registryVendorKeys.packageRegistries,
      queryFn: () =>
        getJSON<PackageRegistry[]>(
          '/api/v1/package-registries',
          'list package registries',
        ),
    }),
  )
}

export function useCreatePackageRegistry() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (req: CreatePackageRegistryRequest) =>
      sendJSON<PackageRegistry>(
        '/api/v1/package-registries',
        'POST',
        'create package registry',
        req,
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: registryVendorKeys.packageRegistries,
      })
    },
  })
}

export function useDeletePackageRegistry() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      sendJSON<void>(
        `/api/v1/package-registries/${encodeURIComponent(id)}`,
        'DELETE',
        'delete package registry',
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: registryVendorKeys.packageRegistries,
      })
    },
  })
}

export function useTestPackageRegistry() {
  return useMutation({
    mutationFn: (id: string) =>
      sendJSON<ConnectionTestResult>(
        `/api/v1/package-registries/${encodeURIComponent(id)}/test`,
        'POST',
        'test package registry',
      ),
  })
}
