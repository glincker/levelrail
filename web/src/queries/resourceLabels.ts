import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type LabelKindPath = 'apps' | 'databases' | 'stacks'

export const resourceLabelKeys = {
  one: (path: LabelKindPath, name: string) =>
    ['resource-labels', path, name] as const,
  match: (path: LabelKindPath, selector: string) =>
    ['resource-labels', path, 'match', selector] as const,
}

function labelsUrl(path: LabelKindPath, name: string): string {
  return `/api/v1/${path}/${encodeURIComponent(name)}/labels`
}

async function fail(res: Response, what: string): Promise<ApiError> {
  return new ApiError(
    res.status,
    await readErrorMessage(res, `${what} failed: ${String(res.status)}`),
  )
}

export async function fetchResourceLabels(
  path: LabelKindPath,
  name: string,
): Promise<Record<string, string>> {
  const res = await fetch(labelsUrl(path, name))
  if (!res.ok) throw await fail(res, 'fetch labels')
  const body = (await res.json()) as { labels?: Record<string, string> }
  return body.labels ?? {}
}

export function useResourceLabels(path: LabelKindPath, name: string) {
  return useQuery({
    queryKey: resourceLabelKeys.one(path, name),
    queryFn: () => fetchResourceLabels(path, name),
    retry: false,
  })
}

export function useSetResourceLabels(path: LabelKindPath, name: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (labels: Record<string, string>) => {
      const res = await fetch(labelsUrl(path, name), {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ labels }),
      })
      if (!res.ok) throw await fail(res, 'save labels')
      const body = (await res.json()) as { labels?: Record<string, string> }
      return body.labels ?? {}
    },
    onSuccess: (labels) => {
      queryClient.setQueryData(resourceLabelKeys.one(path, name), labels)
      void queryClient.invalidateQueries({ queryKey: ['resource-labels'] })
    },
  })
}

// Names matching a label selector, asked of the server so the grammar stays in one place.
export function useLabelMatches(path: 'apps' | 'databases', selector: string) {
  const trimmed = selector.trim()
  return useQuery({
    queryKey: resourceLabelKeys.match(path, trimmed),
    enabled: trimmed !== '',
    retry: false,
    queryFn: async () => {
      const qs = new URLSearchParams({ label: trimmed, limit: '1000' })
      const res = await fetch(`/api/v1/${path}?${qs.toString()}`)
      if (!res.ok) throw await fail(res, 'filter by label')
      const rows = (await res.json()) as { name: string }[]
      return new Set(rows.map((r) => r.name))
    },
  })
}
