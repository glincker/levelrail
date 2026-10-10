import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type {
  CreateDatabaseUserInput,
  CreatedDatabaseUser,
  DatabaseCredential,
  DatabaseNetwork,
  DatabaseUser,
  DatabaseWho,
  GrantInput,
  GrantResult,
  IssuedTemp,
  NetworkScope,
  RulesInput,
  RulesPlan,
  ScopeResult,
  TempList,
  TempPreset,
} from '../types/databaseAccess'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { databaseKeys } from './databases'

export const databaseAccessKeys = {
  all: (name: string) => [...databaseKeys.detail(name), 'access'] as const,
  users: (name: string) => [...databaseAccessKeys.all(name), 'users'] as const,
  temp: (name: string) => [...databaseAccessKeys.all(name), 'temp'] as const,
  who: (name: string) => [...databaseAccessKeys.all(name), 'who'] as const,
  network: (name: string) =>
    [...databaseAccessKeys.all(name), 'network'] as const,
}

const base = (name: string) => `/api/v1/databases/${encodeURIComponent(name)}`

async function request<T>(
  url: string,
  init: RequestInit | undefined,
  label: string,
): Promise<T> {
  const res = await fetch(url, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${label} failed: ${res.status}`),
    )
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

function json(method: string, body?: unknown): RequestInit {
  return {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  }
}

export function useDatabaseUsers(name: string) {
  return useQuery({
    queryKey: databaseAccessKeys.users(name),
    queryFn: () =>
      request<DatabaseUser[]>(`${base(name)}/users`, undefined, 'list users'),
    staleTime: 10_000,
    retry: false,
  })
}

export function useDatabaseTemp(name: string) {
  return useQuery({
    queryKey: databaseAccessKeys.temp(name),
    queryFn: () =>
      request<TempList>(
        `${base(name)}/access/temp`,
        undefined,
        'list temporary logins',
      ),
    refetchInterval: 30_000,
  })
}

export function useDatabaseWho(name: string) {
  return useQuery({
    queryKey: databaseAccessKeys.who(name),
    queryFn: () =>
      request<DatabaseWho>(
        `${base(name)}/access/principals`,
        undefined,
        'list principals',
      ),
  })
}

export function useDatabaseNetwork(name: string) {
  return useQuery({
    queryKey: databaseAccessKeys.network(name),
    queryFn: () =>
      request<DatabaseNetwork>(`${base(name)}/network`, undefined, 'network'),
    staleTime: 10_000,
  })
}

function useInvalidateAccess(name: string) {
  const qc = useQueryClient()
  return () => qc.invalidateQueries({ queryKey: databaseAccessKeys.all(name) })
}

export function useCreateDatabaseUser(name: string) {
  const invalidate = useInvalidateAccess(name)
  return useMutation<CreatedDatabaseUser, ApiError, CreateDatabaseUserInput>({
    mutationFn: (input) =>
      request(`${base(name)}/users`, json('POST', input), 'create user'),
    onSuccess: invalidate,
  })
}

export function useRotateDatabaseUser(name: string) {
  const invalidate = useInvalidateAccess(name)
  return useMutation<DatabaseCredential, ApiError, string>({
    mutationFn: (role) =>
      request(
        `${base(name)}/users/${encodeURIComponent(role)}/rotate`,
        json('POST'),
        'rotate password',
      ),
    onSuccess: invalidate,
  })
}

export function useSetDatabaseUserLogin(name: string) {
  const invalidate = useInvalidateAccess(name)
  return useMutation<void, ApiError, { role: string; login: boolean }>({
    mutationFn: ({ role, login }) =>
      request(
        `${base(name)}/users/${encodeURIComponent(role)}/${login ? 'enable' : 'disable'}`,
        json('POST'),
        'change login',
      ),
    onSuccess: invalidate,
  })
}

export function useDeleteDatabaseUser(name: string) {
  const invalidate = useInvalidateAccess(name)
  return useMutation<void, ApiError, string>({
    mutationFn: (role) =>
      request(
        `${base(name)}/users/${encodeURIComponent(role)}`,
        { method: 'DELETE' },
        'delete user',
      ),
    onSuccess: invalidate,
  })
}

export function useIssueTemp(name: string) {
  const invalidate = useInvalidateAccess(name)
  return useMutation<
    IssuedTemp,
    ApiError,
    { preset: TempPreset; ttl_minutes: number }
  >({
    mutationFn: (input) =>
      request(
        `${base(name)}/access/temp`,
        json('POST', input),
        'issue temporary login',
      ),
    onSuccess: invalidate,
  })
}

export function useRevokeTemp(name: string) {
  const invalidate = useInvalidateAccess(name)
  return useMutation<void, ApiError, string>({
    mutationFn: (id) =>
      request(
        `${base(name)}/access/temp/${encodeURIComponent(id)}`,
        { method: 'DELETE' },
        'revoke temporary login',
      ),
    onSuccess: invalidate,
  })
}

export function useGrantAccess(name: string) {
  const invalidate = useInvalidateAccess(name)
  return useMutation<GrantResult, ApiError, GrantInput>({
    mutationFn: (input) =>
      request(`${base(name)}/access/grants`, json('POST', input), 'grant'),
    onSuccess: (res) => {
      if (res.applied) void invalidate()
    },
  })
}

export function usePreviewRules(name: string) {
  return useMutation<RulesPlan, ApiError, RulesInput>({
    mutationFn: (input) =>
      request(
        `${base(name)}/network/rules/preview`,
        json('POST', input),
        'preview rules',
      ),
  })
}

export function useApplyRules(name: string) {
  const invalidate = useInvalidateAccess(name)
  return useMutation<RulesPlan, ApiError, RulesInput>({
    mutationFn: (input) =>
      request(
        `${base(name)}/network/rules`,
        json('PUT', { ...input, confirm: true }),
        'apply rules',
      ),
    onSuccess: invalidate,
  })
}

export function useRemoveRules(name: string) {
  const invalidate = useInvalidateAccess(name)
  return useMutation<void, ApiError, void>({
    mutationFn: () =>
      request(
        `${base(name)}/network/rules`,
        { method: 'DELETE' },
        'remove rules',
      ),
    onSuccess: invalidate,
  })
}

export function useMakePrivate(name: string) {
  const invalidate = useInvalidateAccess(name)
  const qc = useQueryClient()
  return useMutation<void, ApiError, void>({
    mutationFn: () =>
      request(
        `${base(name)}/network/make-private`,
        json('POST'),
        'make private',
      ),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: databaseKeys.detail(name) })
      void invalidate()
    },
  })
}

export function useSetScope(name: string) {
  const invalidate = useInvalidateAccess(name)
  return useMutation<
    ScopeResult,
    ApiError,
    { scope: NetworkScope; dry_run?: boolean; confirm?: boolean }
  >({
    mutationFn: (input) =>
      request(`${base(name)}/network/scope`, json('PUT', input), 'set scope'),
    onSuccess: (res) => {
      if (res.applied) void invalidate()
    },
  })
}

export function useSetRequireTls(name: string) {
  const invalidate = useInvalidateAccess(name)
  return useMutation<void, ApiError, boolean>({
    mutationFn: (require) =>
      request(
        `${base(name)}/network/tls`,
        json('PUT', { require }),
        'set tls requirement',
      ),
    onSuccess: invalidate,
  })
}
