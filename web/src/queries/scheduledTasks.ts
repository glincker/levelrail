// Query-key factory and fetchers for
// GET/POST /api/v1/apps/{name}/scheduled-tasks,
// PUT/DELETE .../scheduled-tasks/{id}, POST .../scheduled-tasks/{id}/run,
// and GET .../scheduled-tasks/{id}/runs (internal/api/scheduled_tasks.go).
// Same "plain useQuery, not loader-primed" shape queries/alerts.ts already
// establishes: this is a section on the app detail page, not something
// the route loader needs warm before first paint.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import type {
  ScheduledTask,
  ScheduledTaskRequest,
  ScheduledTaskRun,
} from '../types/scheduledTasks'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const scheduledTaskKeys = {
  all: (appName: string) => [...appKeys.detail(appName), 'scheduled-tasks'] as const,
  list: (appName: string) => [...scheduledTaskKeys.all(appName), 'list'] as const,
  runs: (appName: string, taskId: string) =>
    [...scheduledTaskKeys.all(appName), taskId, 'runs'] as const,
}

export async function fetchScheduledTasks(appName: string): Promise<ScheduledTask[]> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(appName)}/scheduled-tasks`)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch scheduled tasks failed: ${res.status}`),
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
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/scheduled-tasks`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `create scheduled task failed: ${res.status}`),
    )
  }
  return (await res.json()) as ScheduledTask
}

export function useCreateScheduledTask(appName: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (req: ScheduledTaskRequest) => createScheduledTask(appName, req),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: scheduledTaskKeys.list(appName) })
    },
  })
}

export async function updateScheduledTask(
  appName: string,
  id: string,
  req: ScheduledTaskRequest,
): Promise<ScheduledTask> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/scheduled-tasks/${encodeURIComponent(id)}`,
    {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `update scheduled task failed: ${res.status}`),
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
      void queryClient.invalidateQueries({ queryKey: scheduledTaskKeys.list(appName) })
    },
  })
}

export async function deleteScheduledTask(appName: string, id: string): Promise<void> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/scheduled-tasks/${encodeURIComponent(id)}`,
    { method: 'DELETE' },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `delete scheduled task failed: ${res.status}`),
    )
  }
}

export function useDeleteScheduledTask(appName: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => deleteScheduledTask(appName, id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: scheduledTaskKeys.list(appName) })
    },
  })
}

// runScheduledTask calls POST .../scheduled-tasks/{id}/run: returns as
// soon as the attempt is recorded and under way, not once the command
// finishes (202, matching handleRunScheduledTask's own doc comment).
export async function runScheduledTask(appName: string, id: string): Promise<ScheduledTaskRun> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/scheduled-tasks/${encodeURIComponent(id)}/run`,
    { method: 'POST' },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `run scheduled task failed: ${res.status}`),
    )
  }
  return (await res.json()) as ScheduledTaskRun
}

export function useRunScheduledTask(appName: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => runScheduledTask(appName, id),
    onSuccess: (_run, id) => {
      void queryClient.invalidateQueries({ queryKey: scheduledTaskKeys.runs(appName, id) })
    },
  })
}

export async function fetchScheduledTaskRuns(
  appName: string,
  taskId: string,
): Promise<ScheduledTaskRun[]> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/scheduled-tasks/${encodeURIComponent(taskId)}/runs`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch scheduled task runs failed: ${res.status}`),
    )
  }
  return (await res.json()) as ScheduledTaskRun[]
}

export function scheduledTaskRunsQueryOptions(appName: string, taskId: string) {
  return queryOptions({
    queryKey: scheduledTaskKeys.runs(appName, taskId),
    queryFn: () => fetchScheduledTaskRuns(appName, taskId),
  })
}

export function useScheduledTaskRuns(appName: string, taskId: string, enabled: boolean) {
  return useQuery({ ...scheduledTaskRunsQueryOptions(appName, taskId), enabled })
}
