// Writes for connection bindings: POST and DELETE /api/v1/apps/{name}/bindings.

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { appDeliveryKeys } from './appDelivery'
import { connectionKeys } from './connections'
import type { AppBinding } from '../types/appDelivery'

export interface BindInput {
  app: string
  ref: string
  role: string
  /** Raw JSON object text; empty means no rules. */
  rules: string
}

export interface UnbindInput {
  app: string
  id: string
}

function bindingsUrl(app: string): string {
  return `/api/v1/apps/${encodeURIComponent(app)}/bindings`
}

async function bindConnection({
  app,
  ref,
  role,
  rules,
}: BindInput): Promise<AppBinding> {
  const body: Record<string, unknown> = { ref, role }
  if (rules.trim() !== '') body.rules = JSON.parse(rules) as unknown
  const res = await fetch(bindingsUrl(app), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `bind connection failed: ${res.status}`),
    )
  }
  return (await res.json()) as AppBinding
}

async function unbindConnection({ app, id }: UnbindInput): Promise<void> {
  const res = await fetch(`${bindingsUrl(app)}/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `unbind connection failed: ${res.status}`),
    )
  }
}

// Both changes move the Delivery strip and the bound counts on the cards.
function useRefresh() {
  const queryClient = useQueryClient()
  return () => {
    void queryClient.invalidateQueries({ queryKey: appDeliveryKeys.all })
    void queryClient.invalidateQueries({ queryKey: connectionKeys.all })
  }
}

export function useBindConnection() {
  const refresh = useRefresh()
  return useMutation({
    meta: { errorToast: false },
    mutationFn: bindConnection,
    onSuccess: refresh,
  })
}

export function useUnbindConnection() {
  const refresh = useRefresh()
  return useMutation({
    meta: { errorToast: false },
    mutationFn: unbindConnection,
    onSuccess: refresh,
  })
}
