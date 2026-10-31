import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { Incident, IncidentDraftInput } from '../types/incident'

export const incidentKeys = {
  all: ['incidents'] as const,
  list: () => [...incidentKeys.all, 'list'] as const,
  detail: (id: string) => [...incidentKeys.all, 'detail', id] as const,
}

// 403/404/501 mean incidents are off or the caller lacks read:sensitive; null hides the view.
export async function fetchIncidents(): Promise<Incident[] | null> {
  const res = await fetch('/api/v1/incidents')
  if (res.status === 403 || res.status === 404 || res.status === 501)
    return null
  if (!res.ok) throw new Error(`fetch incidents failed: ${res.status}`)
  return (await res.json()) as Incident[]
}

export async function fetchIncident(id: string): Promise<Incident> {
  const res = await fetch(`/api/v1/incidents/${encodeURIComponent(id)}`)
  if (!res.ok) throw new Error(`fetch incident failed: ${res.status}`)
  return (await res.json()) as Incident
}

export function useIncidents() {
  return useQuery({
    queryKey: incidentKeys.list(),
    queryFn: fetchIncidents,
    retry: false,
    refetchInterval: 30_000,
  })
}

export function useIncident(id: string | null) {
  return useQuery({
    queryKey: incidentKeys.detail(id ?? ''),
    queryFn: () => fetchIncident(id ?? ''),
    enabled: id !== null,
    retry: false,
  })
}

async function send(
  path: string,
  method: 'POST' | 'PUT',
  body?: unknown,
): Promise<void> {
  const res = await fetch(`/api/v1/incidents/${path}`, {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `incident request failed: ${res.status}`),
    )
  }
}

function useIncidentMutation<V>(run: (v: V) => Promise<void>) {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, V>({
    mutationFn: run,
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: incidentKeys.all })
    },
  })
}

export const useAckIncident = () =>
  useIncidentMutation((id: string) =>
    send(`${encodeURIComponent(id)}/ack`, 'POST'),
  )

export const useResolveIncident = () =>
  useIncidentMutation((id: string) =>
    send(`${encodeURIComponent(id)}/resolve`, 'POST'),
  )

export const useAddIncidentNote = () =>
  useIncidentMutation((v: { id: string; body: string }) =>
    send(`${encodeURIComponent(v.id)}/notes`, 'POST', { body: v.body }),
  )

export const useSaveIncidentDraft = () =>
  useIncidentMutation((v: { id: string; draft: IncidentDraftInput }) =>
    send(`${encodeURIComponent(v.id)}/draft`, 'PUT', v.draft),
  )

export const usePublishIncident = () =>
  useIncidentMutation((v: { id: string; updatedAt: string }) =>
    send(`${encodeURIComponent(v.id)}/publish`, 'POST', {
      updated_at: v.updatedAt,
    }),
  )
