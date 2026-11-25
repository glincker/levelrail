import { useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { databaseKeys } from './databases'

export type IntegrationStateCode =
  | 'connected'
  | 'available'
  | 'needs_setup'
  | 'docs_only'
  | 'unavailable'
  | 'planned'

// Wire shape of GET /api/v1/databases/{name}/integrations (internal/dbintegrations).
export interface IntegrationCard {
  id: string
  category: string
  title: string
  summary: string
  engines: string[]
  kind: 'connector' | 'docs'
  docs: string
  docs_planned: boolean
  featured: boolean
  state: { code: IntegrationStateCode; reason?: string }
}

export interface DatabaseIntegrations {
  database: string
  engine: string
  integrations: IntegrationCard[]
}

export function useDatabaseIntegrations(name: string) {
  return useQuery({
    queryKey: [...databaseKeys.detail(name), 'integrations'] as const,
    queryFn: async () => {
      const res = await fetch(
        `/api/v1/databases/${encodeURIComponent(name)}/integrations`,
      )
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `integrations failed: ${res.status}`),
        )
      }
      return (await res.json()) as DatabaseIntegrations
    },
    staleTime: 30_000,
  })
}
