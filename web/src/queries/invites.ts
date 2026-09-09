// Fetchers and hooks for internal/api/invites.go: the team-invite flow
// on the Users settings page, additive on top of createUser/deleteUser
// in queries/users.ts.

import {
  queryOptions,
  useMutation,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { setStoredUsername } from '../lib/authStore'
import type { Ability } from '../types/token'

export interface InviteResource {
  id: string
  email: string
  // role is the curated preset (internal/api/roles.go) this invite was
  // created with, absent when Abilities was hand-picked instead.
  role?: string
  abilities: Ability[]
  created_by?: string
  created_at: string
  expires_at: string
  // expired is computed server-side from expires_at, not a status this
  // client derives itself, so it always agrees with the server's clock.
  expired: boolean
}

// CreateInviteRequest's abilities/role are mutually exclusive, same
// precedence CreateUserRequest documents.
export interface CreateInviteRequest {
  email: string
  role?: string
  abilities?: Ability[]
}

// CreateInviteResponse is create's own response shape: the invite plus
// link, the accept URL to copy/paste or forward by hand. Always present
// regardless of whether email delivery is configured (handleCreateInvite's
// own doc comment): a control plane with no SMTP set up is still fully
// usable through this field.
export interface CreateInviteResponse extends InviteResource {
  link: string
}

export const inviteKeys = {
  all: ['invites'] as const,
  list: () => [...inviteKeys.all, 'list'] as const,
}

export async function fetchInvites(): Promise<InviteResource[]> {
  const res = await fetch('/api/v1/invites')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch invites failed: ${res.status}`),
    )
  }
  return (await res.json()) as InviteResource[]
}

export function inviteListQueryOptions() {
  return queryOptions({ queryKey: inviteKeys.list(), queryFn: fetchInvites })
}

export function useInvites() {
  return useSuspenseQuery(inviteListQueryOptions())
}

// POST /api/v1/invites: AbilityRoot-gated (internal/api's
// handleCreateInvite doc comment), same tier as createUser: the caller
// picks the invited abilities, so only a root caller may hand out any
// subset of them.
export async function createInvite(
  req: CreateInviteRequest,
): Promise<CreateInviteResponse> {
  const res = await fetch('/api/v1/invites', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `create invite failed: ${res.status}`),
    )
  }
  return (await res.json()) as CreateInviteResponse
}

export function useCreateInvite() {
  const queryClient = useQueryClient()
  return useMutation<CreateInviteResponse, ApiError, CreateInviteRequest>({
    mutationFn: createInvite,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: inviteKeys.list() })
    },
  })
}

export async function revokeInvite(id: string): Promise<void> {
  const res = await fetch(`/api/v1/invites/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `revoke invite failed: ${res.status}`),
    )
  }
}

export function useRevokeInvite() {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, string>({
    mutationFn: revokeInvite,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: inviteKeys.list() })
    },
  })
}

export interface AcceptInviteRequest {
  token: string
  password: string
}

export interface AcceptInviteResult {
  username: string
}

// POST /api/v1/invites/accept: public, unauthenticated, gated by
// possession of the token (internal/api's handleAcceptInvite doc
// comment). Sets a session cookie on success, the same
// {"username": "..."} shape login/register share (queries/auth.ts).
export async function acceptInvite(
  req: AcceptInviteRequest,
): Promise<AcceptInviteResult> {
  const res = await fetch('/api/v1/invites/accept', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ token: req.token, password: req.password }),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `accept invite failed: ${res.status}`),
    )
  }
  return (await res.json()) as AcceptInviteResult
}

// Same "record the username, navigate to /" success shape useRegister
// (queries/auth.ts) already uses: accepting an invite is a self-service
// account creation flow just like registration, it just names the
// account in advance instead of letting the caller choose it.
export function useAcceptInvite() {
  const navigate = useNavigate()
  return useMutation<AcceptInviteResult, ApiError, AcceptInviteRequest>({
    mutationFn: acceptInvite,
    onSuccess: (result) => {
      setStoredUsername(result.username)
      void navigate({ to: '/' })
    },
  })
}
