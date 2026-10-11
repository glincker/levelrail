import {
  queryOptions,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type TeamRole = 'owner' | 'admin' | 'developer' | 'viewer'

export const TEAM_ROLES: TeamRole[] = ['owner', 'admin', 'developer', 'viewer']

export interface TeamResource {
  id: string
  name: string
  default: boolean
  role: TeamRole | ''
  member_count: number
  created_at: string
}

export interface TeamMemberResource {
  user_id: string
  email: string
  display_name: string
  role: TeamRole
  created_at: string
}

export interface TeamInviteResource {
  id: string
  team_id: string
  email: string
  role: TeamRole
  created_at: string
  expires_at: string
  status: 'pending' | 'accepted' | 'revoked' | 'expired'
  token?: string
  link?: string
}

export interface TeamEventResource {
  id: string
  actor_name: string
  action: string
  subject: string
  detail: string
  created_at: string
}

export interface CreateTeamInviteInput {
  email: string
  role: TeamRole
  expires_in_hours?: number
}

export const teamKeys = {
  all: ['teams'] as const,
  list: () => [...teamKeys.all, 'list'] as const,
  members: (id: string) => [...teamKeys.all, id, 'members'] as const,
  invites: (id: string) => [...teamKeys.all, id, 'invites'] as const,
  events: (id: string) => [...teamKeys.all, id, 'events'] as const,
}

async function request<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const res = await fetch(path, {
    method,
    headers:
      body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${method} ${path} failed: ${res.status}`),
    )
  }
  if (res.status === 204) {
    return undefined as T
  }
  return (await res.json()) as T
}

const teamPath = (id: string) => `/api/v1/teams/${encodeURIComponent(id)}`

export function teamListQueryOptions() {
  return queryOptions({
    queryKey: teamKeys.list(),
    queryFn: () => request<TeamResource[]>('GET', '/api/v1/teams'),
  })
}

export function teamMembersQueryOptions(id: string) {
  return queryOptions({
    queryKey: teamKeys.members(id),
    queryFn: () =>
      request<TeamMemberResource[]>('GET', `${teamPath(id)}/members`),
    enabled: id !== '',
  })
}

export function teamInvitesQueryOptions(id: string, enabled: boolean) {
  return queryOptions({
    queryKey: teamKeys.invites(id),
    queryFn: () =>
      request<TeamInviteResource[]>('GET', `${teamPath(id)}/invites`),
    enabled: enabled && id !== '',
  })
}

export function teamEventsQueryOptions(id: string, enabled: boolean) {
  return queryOptions({
    queryKey: teamKeys.events(id),
    queryFn: () =>
      request<TeamEventResource[]>('GET', `${teamPath(id)}/events?limit=50`),
    enabled: enabled && id !== '',
  })
}

function useTeamMutation<TVars, TOut = void>(
  fn: (vars: TVars) => Promise<TOut>,
) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: fn,
    onSuccess: () => qc.invalidateQueries({ queryKey: teamKeys.all }),
  })
}

export function useCreateTeam() {
  return useTeamMutation((name: string) =>
    request<TeamResource>('POST', '/api/v1/teams', { name }),
  )
}

export function useSetMemberRole(teamId: string) {
  return useTeamMutation((v: { userId: string; role: TeamRole }) =>
    request<void>(
      'PUT',
      `${teamPath(teamId)}/members/${encodeURIComponent(v.userId)}`,
      { role: v.role },
    ),
  )
}

export function useRemoveMember(teamId: string) {
  return useTeamMutation((userId: string) =>
    request<void>(
      'DELETE',
      `${teamPath(teamId)}/members/${encodeURIComponent(userId)}`,
    ),
  )
}

export function useTransferTeam(teamId: string) {
  return useTeamMutation((userId: string) =>
    request<void>('POST', `${teamPath(teamId)}/transfer`, { user_id: userId }),
  )
}

export function useCreateTeamInvite(teamId: string) {
  return useTeamMutation((input: CreateTeamInviteInput) =>
    request<TeamInviteResource>('POST', `${teamPath(teamId)}/invites`, input),
  )
}

export function useRevokeTeamInvite(teamId: string) {
  return useTeamMutation((inviteId: string) =>
    request<void>(
      'DELETE',
      `${teamPath(teamId)}/invites/${encodeURIComponent(inviteId)}`,
    ),
  )
}

export function useAcceptTeamInvite() {
  return useTeamMutation(
    (input: {
      token: string
      email?: string
      password?: string
      display_name?: string
    }) =>
      request<{ team_id: string; role: TeamRole }>(
        'POST',
        '/api/v1/team-invites/accept',
        input,
      ),
  )
}
