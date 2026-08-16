// Fetchers and hooks for internal/api/users.go: the read-only Users
// settings page, plus create/delete.

import {
  queryOptions,
  useMutation,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface UserResource {
  id: string
  email: string
  display_name: string
  has_password: boolean
  providers: string[]
  is_first_user: boolean
  created_at: string
  last_login_at?: string
}

export interface CreateUserRequest {
  email: string
  display_name?: string
  password: string
}

export const userKeys = {
  all: ['users'] as const,
  list: () => [...userKeys.all, 'list'] as const,
}

export async function fetchUsers(): Promise<UserResource[]> {
  const res = await fetch('/api/v1/users')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch users failed: ${res.status}`),
    )
  }
  return (await res.json()) as UserResource[]
}

export function userListQueryOptions() {
  return queryOptions({ queryKey: userKeys.list(), queryFn: fetchUsers })
}

export function useUsers() {
  return useSuspenseQuery(userListQueryOptions())
}

// POST /api/v1/auth/users: any authenticated user may create another
// local-password user (internal/api/handleCreateUser's own doc comment).
export async function createUser(
  req: CreateUserRequest,
): Promise<UserResource> {
  const res = await fetch('/api/v1/auth/users', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `create user failed: ${res.status}`),
    )
  }
  return (await res.json()) as UserResource
}

export function useCreateUser() {
  const queryClient = useQueryClient()
  return useMutation<UserResource, ApiError, CreateUserRequest>({
    mutationFn: createUser,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: userKeys.list() })
    },
  })
}

export async function deleteUser(id: string): Promise<void> {
  const res = await fetch(`/api/v1/users/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `remove user failed: ${res.status}`),
    )
  }
}

export function useDeleteUser() {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, string>({
    mutationFn: deleteUser,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: userKeys.list() })
    },
  })
}
