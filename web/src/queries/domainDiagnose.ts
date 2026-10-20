// Domain diagnosis: GET /api/v1/domains/{domain}/diagnose/stream sends each
// check as it finishes (DNS first), then one "done" message with the ordered
// report. Mirrors internal/domaindoctor's JSON.

import { useCallback, useEffect, useRef, useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type DiagnoseState = 'pass' | 'warn' | 'fail' | 'skipped' | 'unavailable'

export type DiagnoseFixKind = 'auto' | 'guided' | 'manual'

export interface DiagnoseRecord {
  name: string
  type: string
  value: string
}

export interface DiagnoseAction {
  kind: 'request' | 'link' | 'copy'
  label: string
  api?: string
  value?: string
  body?: Record<string, string>
}

export interface DiagnoseFix {
  summary: string
  kind: DiagnoseFixKind
  action?: DiagnoseAction
  records?: DiagnoseRecord[]
  cli?: string
}

export interface DiagnoseCheck {
  id: string
  title: string
  state: DiagnoseState
  tier: number
  detail?: string
  fix?: DiagnoseFix
}

export type PropagationState = 'settled' | 'propagating' | 'waiting'

export interface DiagnosePropagation {
  state: PropagationState
  detail: string
  next_check_seconds?: number
}

export interface DiagnoseReport {
  domain: string
  app?: string
  checked_at: string
  status: 'healthy' | 'warnings' | 'problems'
  probed: boolean
  probe_note?: string
  checks: DiagnoseCheck[]
  records?: DiagnoseRecord[]
  propagation?: DiagnosePropagation
  go_live?: string
}

type DiagnoseMessage =
  | { type: 'check'; check: DiagnoseCheck }
  | { type: 'done'; report: DiagnoseReport }

export const diagnoseKeys = {
  all: ['domains', 'diagnose'] as const,
}

/** parseDiagnoseLines splits SSE text into messages, returning the unread tail. */
export function parseDiagnoseChunk(buffer: string): {
  messages: DiagnoseMessage[]
  rest: string
} {
  const parts = buffer.split('\n\n')
  const rest = parts.pop() ?? ''
  const messages: DiagnoseMessage[] = []
  for (const part of parts) {
    const line = part.split('\n').find((l) => l.startsWith('data: '))
    if (!line) continue
    try {
      messages.push(JSON.parse(line.slice(6)) as DiagnoseMessage)
    } catch {
      continue
    }
  }
  return { messages, rest }
}

export async function streamDiagnosis(
  domain: string,
  onCheck: (check: DiagnoseCheck) => void,
  signal: AbortSignal,
): Promise<DiagnoseReport> {
  const res = await fetch(
    `/api/v1/domains/${encodeURIComponent(domain)}/diagnose/stream`,
    { signal, headers: { Accept: 'text/event-stream' } },
  )
  if (!res.ok || !res.body) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `diagnose domain failed: ${res.status}`),
    )
  }
  const reader = res.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  for (;;) {
    const { done, value } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })
    const parsed = parseDiagnoseChunk(buffer)
    buffer = parsed.rest
    for (const msg of parsed.messages) {
      if (msg.type === 'check') onCheck(msg.check)
      else return msg.report
    }
  }
  throw new ApiError(502, 'The diagnosis ended before it finished.')
}

export interface DiagnosisRun {
  checks: DiagnoseCheck[]
  report: DiagnoseReport | undefined
  running: boolean
  error: ApiError | undefined
  run: () => void
}

const MAX_AUTO_RECHECKS = 10

/** useDomainDiagnosis streams a diagnosis and re-runs it on the server's next-check schedule while DNS has not settled. */
export function useDomainDiagnosis(domain: string, auto = true): DiagnosisRun {
  const [checks, setChecks] = useState<DiagnoseCheck[]>([])
  const [report, setReport] = useState<DiagnoseReport>()
  const [running, setRunning] = useState(auto)
  const [error, setError] = useState<ApiError>()
  const abortRef = useRef<AbortController>(undefined)
  const rechecks = useRef(0)

  const begin = useCallback(
    (ctrl: AbortController) => {
      streamDiagnosis(
        domain,
        (c) => {
          setChecks((prev) => [...prev.filter((p) => p.id !== c.id), c])
        },
        ctrl.signal,
      )
        .then((r) => {
          setReport(r)
          setChecks(r.checks)
          setRunning(false)
        })
        .catch((e: unknown) => {
          if (ctrl.signal.aborted) return
          setRunning(false)
          setError(
            e instanceof ApiError
              ? e
              : new ApiError(0, e instanceof Error ? e.message : String(e)),
          )
        })
    },
    [domain],
  )

  const run = useCallback(() => {
    abortRef.current?.abort()
    const ctrl = new AbortController()
    abortRef.current = ctrl
    setRunning(true)
    setError(undefined)
    setChecks([])
    begin(ctrl)
  }, [begin])

  useEffect(() => {
    if (!auto) return undefined
    rechecks.current = 0
    const ctrl = new AbortController()
    abortRef.current = ctrl
    begin(ctrl)
    return () => {
      ctrl.abort()
    }
  }, [auto, begin])

  const nextSeconds = report?.propagation?.next_check_seconds ?? 0
  useEffect(() => {
    if (running || nextSeconds <= 0 || rechecks.current >= MAX_AUTO_RECHECKS) {
      return undefined
    }
    const id = setTimeout(() => {
      rechecks.current += 1
      run()
    }, nextSeconds * 1000)
    return () => {
      clearTimeout(id)
    }
  }, [running, nextSeconds, report, run])

  const recheck = useCallback(() => {
    rechecks.current = 0
    run()
  }, [run])

  return { checks, report, running, error, run: recheck }
}

/** applyDiagnoseAction runs an action of kind "request": its api string is "METHOD /path". */
export async function applyDiagnoseAction(
  action: DiagnoseAction,
): Promise<void> {
  const [method, path] = (action.api ?? '').split(' ')
  if (!method || !path?.startsWith('/api/v1/')) {
    throw new ApiError(400, 'This fix cannot be applied from here.')
  }
  const res = await fetch(path, {
    method,
    headers: action.body ? { 'Content-Type': 'application/json' } : undefined,
    body: action.body ? JSON.stringify(action.body) : undefined,
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${action.label} failed: ${res.status}`),
    )
  }
}

export function useApplyDiagnoseAction() {
  return useMutation<void, ApiError, DiagnoseAction>({
    mutationFn: applyDiagnoseAction,
  })
}
