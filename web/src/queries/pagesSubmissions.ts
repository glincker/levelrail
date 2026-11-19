import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { staticSitesKeys } from './staticSites'

export interface SiteSubmission {
  id: number
  form: string
  fields: Record<string, string>
  spam: boolean
  spam_reason?: string
  created_at: string
}

const root = (name: string) =>
  `/api/v1/static-sites/${encodeURIComponent(name)}/submissions`

const submissionsKey = (name: string) =>
  [...staticSitesKeys.all, 'submissions', name] as const

export function siteSubmissionsQueryOptions(name: string) {
  return queryOptions({
    queryKey: submissionsKey(name),
    queryFn: async (): Promise<SiteSubmission[]> => {
      const res = await fetch(root(name))
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `submissions failed: ${res.status}`),
        )
      }
      const body = (await res.json()) as { submissions: SiteSubmission[] }
      return body.submissions
    },
    staleTime: 30_000,
    retry: false,
  })
}

export function useSiteSubmissions(name: string) {
  return useQuery(siteSubmissionsQueryOptions(name))
}

export function useDeleteSubmission(name: string) {
  const qc = useQueryClient()
  return useMutation<void, ApiError, { form: string; id: number }>({
    mutationFn: async ({ form, id }) => {
      const res = await fetch(
        `${root(name)}/${encodeURIComponent(form)}/${id}`,
        { method: 'DELETE' },
      )
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `delete failed: ${res.status}`),
        )
      }
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: submissionsKey(name) }),
  })
}
