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
): Promise<OneShotJob> {
  const res = await fetch('/api/v1/jobs/runs', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ app, command, backoff_limit: 0 }),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `run failed: ${res.status}`),
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

export function useOneShotRun() {
  const [job, setJob] = useState<OneShotJob | undefined>()
  const [pollError, setPollError] = useState<string | undefined>()
  const [polling, setPolling] = useState(false)
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
    if (runToken.current === token) setPolling(false)
  }, [])

  const start = useMutation({
    meta: { errorToast: false },
    mutationFn: ({ app, command }: { app: string; command: string[] }) =>
      startOneShotJob(app, command),
    onSuccess: (started) => {
      runToken.current += 1
      setJob(started)
      void poll(started, runToken.current)
    },
  })

  const run = (app: string, command: string[]) => {
    setJob(undefined)
    setPollError(undefined)
    start.mutate({ app, command })
  }

  return {
    run,
    job,
    startError: start.isError ? start.error.message : undefined,
    pollError,
    running: start.isPending || polling,
  }
}
