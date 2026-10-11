// Query-key factory and fetchers for
// GET/POST/PUT/DELETE /api/v1/apps/{name}/jobs[/{id}] and
// POST .../jobs/{id}/run (internal/api/scheduled_tasks.go).
// Kept in its own module for the same reason queries/alerts.ts is: a
// genuinely different resource shape than AppDetail, nested under the
// same app name.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from '@tanstack/react-query'
import type {
  JobRun,
  ScheduledTask,
  ScheduledTaskRequest,
} from '../types/scheduledTasks'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const scheduledTaskKeys = {
  all: (appName: string) =>
    [...appKeys.detail(appName), 'scheduled-tasks'] as const,
  list: (appName: string) =>
    [...scheduledTaskKeys.all(appName), 'list'] as const,
  runs: (appName: string, taskId: string, status: string) =>
    [...scheduledTaskKeys.all(appName), 'runs', taskId, status] as const,
  run: (appName: string, taskId: string, runId: string) =>
    [...scheduledTaskKeys.all(appName), 'run', taskId, runId] as const,
}

// JOB_RUNS_PAGE is how many runs the history table fetches, the cap that
// keeps it under the virtualization threshold.
export const JOB_RUNS_PAGE = 50

export async function fetchJobRuns(
  appName: string,
  taskId: string,
  status: string,
): Promise<JobRun[]> {
  const base = `/api/v1/apps/${encodeURIComponent(appName)}`
  const path = taskId
    ? `${base}/jobs/${encodeURIComponent(taskId)}/runs`
    : `${base}/job-runs`
  const q = new URLSearchParams({ limit: String(JOB_RUNS_PAGE) })
  if (status) q.set('status', status)
  const res = await fetch(`${path}?${q.toString()}`)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch job runs failed: ${res.status}`),
    )
  }
  return (await res.json()) as JobRun[]
}

// useJobRuns polls while the table is open so a running job settles in
// view; taskId '' lists every job of the app.
export function useJobRuns(appName: string, taskId: string, status: string) {
  return useQuery({
    queryKey: scheduledTaskKeys.runs(appName, taskId, status),
    queryFn: () => fetchJobRuns(appName, taskId, status),
    refetchInterval: 5000,
  })
}

export async function fetchJobRun(
  appName: string,
  taskId: string,
  runId: string,
): Promise<JobRun> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/jobs/${encodeURIComponent(taskId)}/runs/${encodeURIComponent(runId)}`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch job run failed: ${res.status}`),
    )
  }
  return (await res.json()) as JobRun
}

export function useJobRun(appName: string, taskId: string, runId: string) {
  return useQuery({
    queryKey: scheduledTaskKeys.run(appName, taskId, runId),
    queryFn: () => fetchJobRun(appName, taskId, runId),
    refetchInterval: (query) =>
      query.state.data?.status === 'running' ? 2000 : false,
  })
}

export async function fetchScheduledTasks(
  appName: string,
): Promise<ScheduledTask[]> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(appName)}/jobs`)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch scheduled tasks failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as ScheduledTask[]
}

export function scheduledTaskListQueryOptions(appName: string) {
  return queryOptions({
    queryKey: scheduledTaskKeys.list(appName),
    queryFn: () => fetchScheduledTasks(appName),
  })
}

export function useScheduledTasks(appName: string) {
  return useQuery(scheduledTaskListQueryOptions(appName))
}

export async function createScheduledTask(
  appName: string,
  req: ScheduledTaskRequest,
): Promise<ScheduledTask> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(appName)}/jobs`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `create scheduled task failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as ScheduledTask
}

export function useCreateScheduledTask(appName: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (req: ScheduledTaskRequest) =>
      createScheduledTask(appName, req),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: scheduledTaskKeys.list(appName),
      })
    },
  })
}

export async function updateScheduledTask(
  appName: string,
  id: string,
  req: ScheduledTaskRequest,
): Promise<ScheduledTask> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/jobs/${encodeURIComponent(id)}`,
    {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `update scheduled task failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as ScheduledTask
}

export function useUpdateScheduledTask(appName: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, req }: { id: string; req: ScheduledTaskRequest }) =>
      updateScheduledTask(appName, id, req),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: scheduledTaskKeys.list(appName),
      })
    },
  })
}

export async function deleteScheduledTask(
  appName: string,
  id: string,
): Promise<void> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/jobs/${encodeURIComponent(id)}`,
    { method: 'DELETE' },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `delete scheduled task failed: ${res.status}`,
      ),
    )
  }
}

export function useDeleteScheduledTask(appName: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => deleteScheduledTask(appName, id),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: scheduledTaskKeys.list(appName),
      })
    },
  })
}

// pollAfterRun re-invalidates the list query a few times after a "run
// now" trigger returns: handleRunScheduledTaskNow (internal/api) answers
// 202 immediately and finishes the actual run in a background goroutine
// (the same async shape POST .../databases/{name}/backups uses), so the
// task's own last_run_* fields only settle a little after this request
// resolves. A short poll burst rather than a persistent refetchInterval:
// most commands finish in well under the 10-minute runner timeout, and
// this keeps the panel simple (no "is a run currently in flight" piece
// of state to track) at the cost of not catching a genuinely slow run
// without a manual refresh.
function pollAfterRun(queryClient: QueryClient, appName: string) {
  const key = scheduledTaskKeys.all(appName)
  for (const delayMs of [2000, 5000, 10000, 20000]) {
    setTimeout(() => {
      void queryClient.invalidateQueries({ queryKey: key })
    }, delayMs)
  }
}

export async function runScheduledTaskNow(
  appName: string,
  id: string,
): Promise<ScheduledTask> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/jobs/${encodeURIComponent(id)}/run`,
    { method: 'POST' },
  )
  if (res.status === 501) {
    throw new ApiError(
      501,
      'scheduled task execution is not configured on this control plane',
    )
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `run scheduled task failed: ${res.status}`),
    )
  }
  return (await res.json()) as ScheduledTask
}

export function useRunScheduledTaskNow(appName: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => runScheduledTaskNow(appName, id),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: scheduledTaskKeys.all(appName),
      })
      pollAfterRun(queryClient, appName)
    },
  })
}
