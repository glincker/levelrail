import { queryOptions, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

// Matches internal/storageinv's volume and container rows.
export interface InventoryVolume {
  name: string
  size_bytes?: number
  created_at?: string
  in_use: boolean
  labels?: Record<string, string>
  owner_app?: string
  database?: string
  is_database: boolean
  orphaned: boolean
  protected: boolean
  protect_reason?: string
}

export interface InventoryContainer {
  id: string
  name: string
  size_bytes?: number
  owner_app?: string
  exited_at?: string
}

export interface StorageInventory {
  node: string
  volumes: InventoryVolume[]
  containers: InventoryContainer[]
  unsupported: boolean
  reason?: string
}

export const storageInventoryKeys = {
  all: ['storage-inventory'] as const,
  list: (node: string) => ['storage-inventory', 'list', node] as const,
}

export function storageInventoryQueryOptions(node: string) {
  return queryOptions({
    queryKey: storageInventoryKeys.list(node),
    queryFn: async ({ signal }) => {
      const res = await fetch(
        `/api/v1/system/storage/inventory?node=${encodeURIComponent(node)}`,
        { signal },
      )
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(
            res,
            `fetch storage inventory failed: ${res.status}`,
          ),
        )
      }
      return (await res.json()) as StorageInventory
    },
  })
}

export function useStorageInventory(node: string) {
  return useQuery(storageInventoryQueryOptions(node))
}
