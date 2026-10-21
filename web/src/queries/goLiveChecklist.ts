// Go live checklist: GET /api/v1/apps/{name}/go-live/checklist, its SSE
// stream and the stored-state badges. Mirrors internal/golive's JSON.

import { useCallback, useEffect, useRef, useState } from 'react'
import {
  queryOptions,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type StepStatus = 'pending' | 'ok' | 'warn' | 'fail' | 'unknown'

export type LiveState =
  | 'not_live'
  | 'waiting_dns'
  | 'waiting_certificate'
  | 'live'
  | 'needs_attention'

export type StepId =
  'app' | 'port' | 'firewall' | 'dns' | 'certificate' | 'live'

export interface ChecklistAction {
  kind: 'fix' | 'request' | 'recheck' | 'link' | 'copy'
  label: string
  fix_id?: string
  api?: string
  body?: Record<string, unknown>
  href?: string
  value?: string
  blocked?: string
}

export interface ChecklistEvidence {
  label: string
  value: string
  copy?: boolean
}

export interface FixPreview {
  fix_id: string
  kind: string
  commands: string[]
  undo: string[]
  persistent: boolean
  note?: string
  refusal?: string
}

export interface ProviderPortRule {
  port: number
  protocol: string
  source: string
}

export interface ProviderGuidance {
  id: string
  name: string
  docs_url: string
  ports: ProviderPortRule[]
  steps: string[]
  note?: string
}

export interface ChecklistRecord {
  name: string
  type: string
  value: string
}

export interface ChecklistStep {
  id: StepId
  title: string
  status: StepStatus
  summary: string
  evidence: ChecklistEvidence[]
  primary?: ChecklistAction
  next_check_at?: string
  docs_url?: string
  records?: ChecklistRecord[]
  firewall_fix?: FixPreview
  provider?: ProviderGuidance
  scope?: string
}

export interface TimelineEvent {
  id: string
  at: string
  kind: string
  title: string
  detail?: string
  failed?: boolean
  future?: boolean
}

export interface GoLiveChecklistData {
  app: string
  domain: string
  state: LiveState
  live: boolean
  next?: string
  url?: string
  steps: ChecklistStep[]
  timeline: TimelineEvent[]
  checked_at: string
}

export interface LiveBadgeEntry {
  app: string
  domain: string
  state: LiveState
  basis: string
}

type ChecklistMessage =
  | { type: 'checklist'; checklist: GoLiveChecklistData }
  | { type: 'done'; reason?: string }

export const goLiveKeys = {
  checklist: (app: string, domain: string) =>
    ['apps', app, 'go-live', domain] as const,
  status: ['go-live', 'status'] as const,
}

function base(app: string): string {
  return `/api/v1/apps/${encodeURIComponent(app)}/go-live`
}

function query(domain: string): string {
  return domain ? `?domain=${encodeURIComponent(domain)}` : ''
}

export function goLiveChecklistQueryOptions(app: string, domain: string) {
  return queryOptions({
    queryKey: goLiveKeys.checklist(app, domain),
    queryFn: async (): Promise<GoLiveChecklistData> => {
      const res = await fetch(`${base(app)}/checklist${query(domain)}`)
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(
            res,
            `go live checklist failed: ${res.status}`,
          ),
        )
      }
      return (await res.json()) as GoLiveChecklistData
    },
    staleTime: 0,
  })
}

export function liveBadgesQueryOptions() {
  return queryOptions({
    queryKey: goLiveKeys.status,
    queryFn: async (): Promise<LiveBadgeEntry[]> => {
      const res = await fetch('/api/v1/go-live/status')
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `go live status failed: ${res.status}`),
        )
      }
      const body = (await res.json()) as { domains: LiveBadgeEntry[] }
      return body.domains
    },
    staleTime: 30_000,
  })
}

export function parseChecklistChunk(buffer: string): {
  messages: ChecklistMessage[]
  rest: string
} {
  const parts = buffer.split('\n\n')
  const rest = parts.pop() ?? ''
  const messages: ChecklistMessage[] = []
  for (const part of parts) {
    const line = part.split('\n').find((l) => l.startsWith('data: '))
    if (!line) continue
    try {
      messages.push(JSON.parse(line.slice(6)) as ChecklistMessage)
    } catch {
      continue
    }
  }
  return { messages, rest }
}

async function readStream(
  app: string,
  domain: string,
  onChecklist: (c: GoLiveChecklistData) => void,
  signal: AbortSignal,
): Promise<void> {
  const res = await fetch(`${base(app)}/stream${query(domain)}`, {
    signal,
    headers: { Accept: 'text/event-stream' },
  })
  if (!res.ok || !res.body) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `go live stream failed: ${res.status}`),
    )
  }
  const reader = res.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  for (;;) {
    const { done, value } = await reader.read()
    if (done) return
    buffer += decoder.decode(value, { stream: true })
    const parsed = parseChecklistChunk(buffer)
    buffer = parsed.rest
    for (const msg of parsed.messages) {
      if (msg.type === 'checklist') onChecklist(msg.checklist)
      else return
    }
  }
}

export interface ChecklistStream {
  checklist: GoLiveChecklistData | undefined
  error: ApiError | undefined
  refresh: () => void
}

/** useChecklistStream keeps the checklist fresh from the SSE stream and restarts it on demand. */
export function useChecklistStream(
  app: string,
  domain: string,
): ChecklistStream {
  const queryClient = useQueryClient()
  const [checklist, setChecklist] = useState<GoLiveChecklistData>()
  const [error, setError] = useState<ApiError>()
  const [nonce, setNonce] = useState(0)
  const abortRef = useRef<AbortController>(undefined)

  useEffect(() => {
    const ctrl = new AbortController()
    abortRef.current = ctrl
    readStream(
      app,
      domain,
      (c) => {
        setError(undefined)
        setChecklist(c)
        queryClient.setQueryData(goLiveKeys.checklist(app, domain), c)
      },
      ctrl.signal,
    ).catch((e: unknown) => {
      if (ctrl.signal.aborted) return
      setError(
        e instanceof ApiError
          ? e
          : new ApiError(0, e instanceof Error ? e.message : String(e)),
      )
    })
    return () => {
      ctrl.abort()
    }
  }, [app, domain, nonce, queryClient])

  const refresh = useCallback(() => {
    setNonce((n) => n + 1)
  }, [])
  return { checklist, error, refresh }
}

/** runAction performs a "METHOD /path" request action, or a fix by id. */
async function send(
  method: string,
  path: string,
  body?: Record<string, unknown>,
): Promise<void> {
  const res = await fetch(path, {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: body ? JSON.stringify(body) : undefined,
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `request failed: ${res.status}`),
    )
  }
}

export function useApplyChecklistAction(onDone: () => void) {
  return useMutation<void, ApiError, ChecklistAction>({
    mutationFn: async (a) => {
      if (a.kind === 'fix' && a.fix_id) {
        await send(
          'POST',
          `/api/v1/fixes/${encodeURIComponent(a.fix_id)}/apply`,
        )
        return
      }
      if (a.kind === 'request' && a.api) {
        const [method, path] = a.api.split(' ')
        if (method && path) await send(method, path, a.body)
      }
    },
    onSuccess: onDone,
  })
}
