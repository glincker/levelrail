import { queryOptions, useQuery } from '@tanstack/react-query'
import { apiRequest } from '../lib/apiRequest'

export interface PagePreview {
  branch: string
  pr_number: number
  site: string
  commit_sha?: string
  status: string
  url?: string
  updated_at: string
}

export const pagePreviewKeys = {
  list: (name: string) => ['static-sites', name, 'previews'] as const,
}

export function pagePreviewsQueryOptions(name: string) {
  return queryOptions({
    queryKey: pagePreviewKeys.list(name),
    queryFn: async ({ signal }): Promise<PagePreview[]> => {
      const body = await apiRequest<{ previews: PagePreview[] } | null>(
        `/api/v1/static-sites/${encodeURIComponent(name)}/previews`,
        { signal, fallbackMessage: 'fetch previews failed' },
      )
      return body?.previews ?? []
    },
  })
}

export function usePagePreviews(name: string) {
  return useQuery(pagePreviewsQueryOptions(name))
}
