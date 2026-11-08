// POST /api/v1/jobs/runs starts a command in a fresh container, GET
// /api/v1/jobs/runs/{id} reports it. Not a cached query: the poll is
// driven by the one card that started the run and stops when it unmounts.

import { useCallback, useEffect, useRef, useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface OneShotJob {
  id: string
  app: string
  state: string
  exit_code?: number
  run_status?: string
  message?: string
  output?: string
  output_truncated?: boolean
}

const POLL_INTERVAL_MS = 1000
// Same tolerance as the CLI: a few blips in a row before giving up.
const MAX_POLL_FAILURES = 3

export async function startOneShotJob(
  app: string,
  command: string[],
  timeoutSeconds = 0,
): Promise<OneShotJob> {
  const res = await fetch('/api/v1/jobs/runs', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      app,
      command,
      backoff_limit: 0,
      ...(timeoutSeconds > 0 ? { timeout_seconds: timeoutSeconds } : {}),
    }),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `run failed: ${res.status}`),
    )
  }
  return (await res.json()) as OneShotJob
}

export async function cancelOneShotJob(id: string): Promise<OneShotJob> {
  const res = await fetch(
    `/api/v1/jobs/runs/${encodeURIComponent(id)}/cancel`,
    {
      method: 'POST',
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `cancel failed: ${res.status}`),
    )
  }
  return (await res.json()) as OneShotJob
}

export async function getOneShotJob(id: string): Promise<OneShotJob> {
  const res = await fetch(`/api/v1/jobs/runs/${encodeURIComponent(id)}`)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `run status failed: ${res.status}`),
    )
  }
  return (await res.json()) as OneShotJob
}

function isRunning(job: OneShotJob): boolean {
  return job.state === 'Running' || job.state === ''
}

const HISTORY_LIMIT = 10

export interface OneShotHistoryEntry {
  job: OneShotJob
  command: string
}

export function useOneShotRun() {
  const [job, setJob] = useState<OneShotJob | undefined>()
  const [pollError, setPollError] = useState<string | undefined>()
  const [polling, setPolling] = useState(false)
  // Settled runs, newest first, in memory only for this page view.
  const [history, setHistory] = useState<OneShotHistoryEntry[]>([])
  const commandLine = useRef('')
  const runToken = useRef(0)

  // Bumping the token on unmount ends any poll loop still in flight.
  useEffect(() => {
    return () => {
      runToken.current += 1
    }
  }, [])

  const poll = useCallback(async (first: OneShotJob, token: number) => {
    let current = first
    let failures = 0
    setPolling(true)
    while (isRunning(current) && runToken.current === token) {
      await new Promise((r) => setTimeout(r, POLL_INTERVAL_MS))
      if (runToken.current !== token) return
      try {
        current = await getOneShotJob(first.id)
        failures = 0
        setJob(current)
      } catch (err) {
        failures += 1
        if (failures >= MAX_POLL_FAILURES) {
          setPollError(err instanceof Error ? err.message : String(err))
          break
        }
      }
    }
    if (runToken.current !== token) return
    setPolling(false)
    if (!isRunning(current)) {
      const settled = current
      setHistory((prev) =>
        [{ job: settled, command: commandLine.current }, ...prev].slice(
          0,
          HISTORY_LIMIT,
        ),
      )
    }
  }, [])

  const start = useMutation({
    meta: { errorToast: false },
    mutationFn: ({
      app,
      command,
      timeout,
    }: {
      app: string
      command: string[]
      timeout: number
    }) => startOneShotJob(app, command, timeout),
    onSuccess: (started) => {
      runToken.current += 1
      setJob(started)
      void poll(started, runToken.current)
    },
  })

  const run = (app: string, command: string[], timeout = 0) => {
    setJob(undefined)
    setPollError(undefined)
    commandLine.current = command.join(' ')
    start.mutate({ app, command, timeout })
  }

  // The server kills and removes the container; the poll loop then reads the settled state.
  const cancel = async () => {
    if (!job?.id) return
    try {
      await cancelOneShotJob(job.id)
    } catch (err) {
      setPollError(err instanceof Error ? err.message : String(err))
    }
  }

  return {
    run,
    cancel,
    history,
    job,
    startForbidden:
      start.isError &&
      start.error instanceof ApiError &&
      start.error.status === 403,
    startError: start.isError ? start.error.message : undefined,
    pollError,
    running: start.isPending || polling,
  }
}
