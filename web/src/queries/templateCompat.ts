// GET /api/v1/templates/compat: templates that failed to install on this host, with fix text.

import { queryOptions, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface TemplateCompatRow {
  template_id: string
  node_arch: string
  class: string
  detail?: string
  failures: number
  first_seen: string
  last_seen: string
  cause: string
  fix: string
}

export const templateCompatKeys = {
  all: ['template-compat'] as const,
}

export function templateCompatQueryOptions() {
  return queryOptions({
    queryKey: templateCompatKeys.all,
    queryFn: async (): Promise<TemplateCompatRow[]> => {
      const res = await fetch('/api/v1/templates/compat')
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(
            res,
            `fetch template compat failed: ${res.status}`,
          ),
        )
      }
      return ((await res.json()) as TemplateCompatRow[] | null) ?? []
    },
    staleTime: 60_000,
  })
}

export function useTemplateCompat() {
  return useQuery(templateCompatQueryOptions())
}
