// Query-key factory and fetcher for GET /api/v1/openapi.json
// (internal/api/openapi.go): route metadata for the API explorer
// (routes/settings/api-explorer.tsx). Session-only (requireAbility
// AbilityRead, routes.go), so this follows the same cookie-only fetch
// convention as every other settings query in this directory.

import { queryOptions, useSuspenseQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

// Mirrors internal/api/openapi.go's openAPISpecRoute exactly.
// requestBody/responseBody are raw JSON examples, present only for the
// handful of routes openAPIExamples hand-annotates server-side.
export interface OpenAPIRoute {
  method: string
  path: string
  ability: string
  group: string
  handler: string
  description?: string
  requestBody?: unknown
  responseBody?: unknown
}

export interface OpenAPISpec {
  version: number
  count: number
  exampleCount: number
  routes: OpenAPIRoute[]
}

export const openAPIKeys = {
  all: ['openapi-spec'] as const,
}

export async function fetchOpenAPISpec(): Promise<OpenAPISpec> {
  const res = await fetch('/api/v1/openapi.json')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch API spec failed: ${res.status}`),
    )
  }
  return (await res.json()) as OpenAPISpec
}

// The spec changes only when the binary is rebuilt (it's generated at
// build time, see scripts/gen-api-reference), so this never needs
// refetching mid-session the way live resource queries do.
export function openAPISpecQueryOptions() {
  return queryOptions({
    queryKey: openAPIKeys.all,
    queryFn: fetchOpenAPISpec,
    staleTime: Infinity,
  })
}

export function useOpenAPISpec() {
  return useSuspenseQuery(openAPISpecQueryOptions())
}
