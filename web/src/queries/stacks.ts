// Fetchers and hooks for the stack endpoints (internal/api/stacks*.go).
// Experimental: only mounted behind the `iac` feature.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type {
  Stack,
  StackDeleteMode,
  StackDeleteResult,
  StackDiff,
  StackDiffAgainst,
  StackHistory,
  StackList,
  StackRevisionDetail,
  StackRollbackResult,
  StackSettingsInput,
} from '../types/stacks'

export const stackKeys = {
  all: ['stacks'] as const,
  list: () => [...stackKeys.all, 'list'] as const,
  detail: (name: string) => [...stackKeys.all, 'detail', name] as const,
  history: (name: string) => [...stackKeys.all, 'history', name] as const,
  revision: (name: string, n: number) =>
    [...stackKeys.all, 'revision', name, n] as const,
  diff: (name: string, n: number, against: StackDiffAgainst) =>
    [...stackKeys.all, 'diff', name, n, against] as const,
}

const base = (name: string) => `/api/v1/stacks/${encodeURIComponent(name)}`

async function request<T>(
  url: string,
  init: RequestInit | undefined,
  what: string,
): Promise<T> {
  const res = await fetch(url, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${String(res.status)}`),
    )
  }
  return (await res.json().catch(() => ({}))) as T
}

function json(method: string, body: unknown): RequestInit {
  return {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }
}

export function stacksQueryOptions() {
  return queryOptions({
    queryKey: stackKeys.list(),
    queryFn: async () =>
      (await request<StackList>('/api/v1/stacks', undefined, 'list stacks'))
        .stacks,
  })
}

export function stackQueryOptions(name: string) {
  return queryOptions({
    queryKey: stackKeys.detail(name),
    queryFn: () => request<Stack>(base(name), undefined, 'load stack'),
  })
}

export function stackHistoryQueryOptions(name: string) {
  return queryOptions({
    queryKey: stackKeys.history(name),
    queryFn: () =>
      request<StackHistory>(
        `${base(name)}/revisions?limit=100`,
        undefined,
        'load revisions',
      ),
  })
}

export function stackRevisionQueryOptions(name: string, n: number) {
  return queryOptions({
    queryKey: stackKeys.revision(name, n),
    queryFn: () =>
      request<StackRevisionDetail>(
        `${base(name)}/revisions/${String(n)}`,
        undefined,
        'load revision',
      ),
  })
}

export function stackDiffQueryOptions(
  name: string,
  n: number,
  against: StackDiffAgainst,
) {
  return queryOptions({
    queryKey: stackKeys.diff(name, n, against),
    queryFn: () =>
      request<StackDiff>(
        `${base(name)}/revisions/${String(n)}/diff?against=${String(against)}`,
        undefined,
        'load diff',
      ),
  })
}

export function useStacks(enabled = true) {
  return useQuery({ ...stacksQueryOptions(), enabled })
}

// Null when the stack is unknown (or the feature is off), so a badge never errors a page.
export function useStackByName(name: string | undefined) {
  return useQuery({
    ...stackQueryOptions(name ?? ''),
    enabled: Boolean(name),
    retry: false,
  })
}

function useInvalidateStacks() {
  const queryClient = useQueryClient()
  return () => queryClient.invalidateQueries({ queryKey: stackKeys.all })
}

export function useRollbackStack(name: string) {
  const invalidate = useInvalidateStacks()
  return useMutation({
    mutationFn: (revision: number) =>
      request<StackRollbackResult>(
        `${base(name)}/revisions/${String(revision)}/rollback`,
        json('POST', {}),
        'roll back',
      ),
    onSuccess: invalidate,
  })
}

export function usePinStack(name: string) {
  const invalidate = useInvalidateStacks()
  return useMutation({
    mutationFn: (revision: number | null) =>
      revision === null
        ? request<unknown>(`${base(name)}/pin`, { method: 'DELETE' }, 'unpin')
        : request<unknown>(
            `${base(name)}/pin`,
            json('PUT', { revision }),
            'pin',
          ),
    onSuccess: invalidate,
  })
}

export function useUnlockStack(name: string) {
  const invalidate = useInvalidateStacks()
  return useMutation({
    mutationFn: () =>
      request<unknown>(`${base(name)}/unlock`, { method: 'POST' }, 'unlock'),
    onSuccess: invalidate,
  })
}

export function useDeleteStack(name: string) {
  const invalidate = useInvalidateStacks()
  return useMutation({
    mutationFn: (mode: StackDeleteMode) =>
      request<StackDeleteResult>(
        `${base(name)}?mode=${mode}`,
        { method: 'DELETE' },
        'delete stack',
      ),
    onSuccess: invalidate,
  })
}

export function useUpdateStackSettings(name: string) {
  const invalidate = useInvalidateStacks()
  return useMutation({
    mutationFn: (input: StackSettingsInput) =>
      request<Stack>(base(name), json('PATCH', input), 'save stack settings'),
    onSuccess: invalidate,
  })
}
