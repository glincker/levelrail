import { useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { databaseKeys } from './databases'
import { useApiMutation } from './useApiMutation'

// Wire shapes of internal/api/database_branches_api.go.
export interface BranchResource {
  name: string
  parent: string
  root: string
  mode: string
  state: string
  state_reason?: string
  protected: boolean
  expires_at?: string
  size_bytes: number
  created_at: string
}

export interface BranchList {
  root: string
  branches: BranchResource[]
}

export interface BranchCapability {
  modes: Record<string, { available: boolean; reason?: string }>
  branches: boolean
}

export interface CreateBranchInput {
  name: string
  mode: string
  parent?: string
  ttl: string
  protected: boolean
}

const base = (name: string) => `/api/v1/databases/${encodeURIComponent(name)}`
const branchKey = (name: string) =>
  [...databaseKeys.detail(name), 'branches'] as const
// Transitional states poll until the clone settles.
const SETTLED = new Set(['ready', 'failed', 'promoted'])
const POLL_MS = Number(import.meta.env.VITE_BRANCH_POLL_MS ?? 4000)

async function call<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `branches failed: ${res.status}`),
    )
  }
  return res.status === 204 ? (undefined as T) : ((await res.json()) as T)
}

export function useDatabaseBranches(name: string) {
  return useQuery({
    queryKey: branchKey(name),
    queryFn: () => call<BranchList>(`${base(name)}/branches`),
    refetchInterval: (q) =>
      q.state.data?.branches.some((b) => !SETTLED.has(b.state))
        ? POLL_MS
        : false,
    retry: false,
  })
}

export function useBranchCapabilities(name: string) {
  return useQuery({
    queryKey: [...branchKey(name), 'capabilities'] as const,
    queryFn: () => call<BranchCapability>(`${base(name)}/branch-capabilities`),
    staleTime: 5 * 60_000,
  })
}

function useBranchMutation<V>(name: string, fn: (v: V) => Promise<unknown>) {
  return useApiMutation<unknown, V>({ fn, invalidate: [branchKey(name)] })
}

const json = (method: string, body: unknown): RequestInit => ({
  method,
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify(body),
})

export function useCreateBranch(name: string) {
  return useBranchMutation(name, (v: CreateBranchInput) =>
    call(`${base(name)}/branches`, json('POST', v)),
  )
}

export function useProtectBranch(name: string) {
  return useBranchMutation(name, (v: { branch: string; protect: boolean }) =>
    call(
      `${base(name)}/branches/${encodeURIComponent(v.branch)}`,
      json('PATCH', { protected: v.protect }),
    ),
  )
}

export type BranchAction = 'reset' | 'promote' | 'delete'

export function useBranchAction(name: string) {
  return useBranchMutation(
    name,
    (v: { branch: string; action: BranchAction }) => {
      const url = `${base(name)}/branches/${encodeURIComponent(v.branch)}`
      return v.action === 'delete'
        ? call(url, json('DELETE', { confirm: v.branch }))
        : call(`${url}/${v.action}`, json('POST', { confirm: v.branch }))
    },
  )
}
