import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { IncidentPostmortem, PostmortemEditInput } from '../types/incident'
import { incidentKeys } from './incidents'

export const postmortemKey = (id: string) =>
  [...incidentKeys.all, 'postmortem', id] as const

const path = (id: string, rest = '') =>
  `/api/v1/incidents/${encodeURIComponent(id)}/postmortem${rest}`

// 404 means no postmortem yet; the section then offers to draft one.
export async function fetchPostmortem(
  id: string,
): Promise<IncidentPostmortem | null> {
  const res = await fetch(path(id))
  if (res.status === 404) return null
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch postmortem failed: ${res.status}`),
    )
  }
  return (await res.json()) as IncidentPostmortem
}

export function usePostmortem(id: string) {
  return useQuery({
    queryKey: postmortemKey(id),
    queryFn: () => fetchPostmortem(id),
    retry: false,
  })
}

async function send(
  url: string,
  method: 'POST' | 'PUT',
  body?: unknown,
): Promise<void> {
  const res = await fetch(url, {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `postmortem request failed: ${res.status}`),
    )
  }
}

function usePostmortemMutation<V>(id: string, run: (v: V) => Promise<void>) {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, V>({
    mutationFn: run,
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: postmortemKey(id) })
    },
  })
}

export const useDraftPostmortem = (id: string) =>
  usePostmortemMutation<void>(id, () => send(path(id, '/draft'), 'POST'))

export const useSavePostmortem = (id: string) =>
  usePostmortemMutation(id, (v: PostmortemEditInput) =>
    send(path(id), 'PUT', v),
  )

export const usePublishPostmortem = (id: string) =>
  usePostmortemMutation<void>(id, () => send(path(id, '/publish'), 'POST'))
