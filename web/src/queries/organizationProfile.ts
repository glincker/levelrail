// Organization home and profile endpoints (internal/api/organization_profile.go):
// summary, activity, PATCH profile and the raw-body avatar PUT/DELETE.

import {
  queryOptions,
  useInfiniteQuery,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import type { OrganizationResource } from '../types/organization'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { organizationKeys } from './organizations'
import { teamKeys } from './teams'

// Mirrors the server defaults (orgs.avatar_max_bytes, 500 rune description);
// the server stays the authority, these only reject early.
export const ORG_DESCRIPTION_MAX = Number(
  import.meta.env.VITE_ORG_DESCRIPTION_MAX ?? 500,
)
export const ORG_AVATAR_MAX_BYTES = Number(
  import.meta.env.VITE_ORG_AVATAR_MAX_BYTES ?? 262144,
)
export const ORG_AVATAR_TYPES = ['image/png', 'image/jpeg'] as const
export const ORG_ACTIVITY_PAGE = Number(
  import.meta.env.VITE_ORG_ACTIVITY_PAGE ?? 20,
)

export interface OrgCounts {
  projects: number
  apps: number
  databases: number
  environments: number
  members: number
  pending_invites: number
}

export interface OrgActivityItem {
  id: string
  actor_id: string
  actor_name: string
  action: string
  subject: string
  detail: string
  created_at: string
}

export interface OrgSummary {
  organization: OrganizationResource
  counts: OrgCounts
  failing_apps: { count: number; names: string[] }
  activity: OrgActivityItem[]
}

export interface OrgActivityPage {
  items: OrgActivityItem[]
  next_cursor?: string
}

export interface OrgProfilePatch {
  name?: string
  slug?: string
  description?: string
}

export const orgHomeKeys = {
  summary: (id: string) => [...organizationKeys.detail(id), 'summary'] as const,
  activity: (id: string) =>
    [...organizationKeys.detail(id), 'activity'] as const,
}

const orgPath = (id: string) =>
  `/api/v1/organizations/${encodeURIComponent(id)}`

async function failIfNotOk(res: Response, what: string): Promise<void> {
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
}

export function orgSummaryQueryOptions(id: string) {
  return queryOptions({
    queryKey: orgHomeKeys.summary(id),
    queryFn: async (): Promise<OrgSummary> => {
      const res = await fetch(`${orgPath(id)}/summary`)
      await failIfNotOk(res, 'fetch organization summary')
      return (await res.json()) as OrgSummary
    },
  })
}

// Only enabled once the user asks for more: the summary already carries the
// newest page, so the first page here overlaps it and the caller dedupes.
export function useOrgActivity(id: string, enabled: boolean) {
  return useInfiniteQuery({
    queryKey: orgHomeKeys.activity(id),
    enabled,
    initialPageParam: '',
    queryFn: async ({ pageParam }): Promise<OrgActivityPage> => {
      const qs = new URLSearchParams({ limit: String(ORG_ACTIVITY_PAGE) })
      if (pageParam !== '') qs.set('cursor', pageParam)
      const res = await fetch(`${orgPath(id)}/activity?${qs.toString()}`)
      await failIfNotOk(res, 'fetch organization activity')
      return (await res.json()) as OrgActivityPage
    },
    getNextPageParam: (last) => last.next_cursor || undefined,
  })
}

export async function patchOrganization(
  id: string,
  patch: OrgProfilePatch,
): Promise<OrganizationResource> {
  const res = await fetch(orgPath(id), {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(patch),
  })
  await failIfNotOk(res, 'update organization')
  return (await res.json()) as OrganizationResource
}

// XHR rather than fetch only because fetch has no upload progress events.
export function putOrganizationAvatar(
  id: string,
  file: Blob,
  onProgress?: (fraction: number) => void,
): Promise<void> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    xhr.open('PUT', `${orgPath(id)}/avatar`)
    xhr.setRequestHeader('Content-Type', file.type)
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable && onProgress) onProgress(e.loaded / e.total)
    }
    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        resolve()
        return
      }
      const text = xhr.responseText.trim()
      let message = text
      try {
        const parsed = JSON.parse(text) as { error?: string }
        message = parsed.error ?? text
      } catch {
        // plain-text body, keep as is
      }
      reject(
        new ApiError(xhr.status, message || `upload failed: ${xhr.status}`),
      )
    }
    xhr.onerror = () => reject(new ApiError(0, 'upload failed: network error'))
    xhr.send(file)
  })
}

export async function deleteOrganizationAvatar(id: string): Promise<void> {
  const res = await fetch(`${orgPath(id)}/avatar`, { method: 'DELETE' })
  await failIfNotOk(res, 'remove organization avatar')
}

function useInvalidateOrg(id: string) {
  const qc = useQueryClient()
  return () => {
    void qc.invalidateQueries({ queryKey: organizationKeys.all })
    void qc.invalidateQueries({ queryKey: teamKeys.list() })
    void qc.invalidateQueries({ queryKey: orgHomeKeys.summary(id) })
    void qc.invalidateQueries({ queryKey: orgHomeKeys.activity(id) })
  }
}

export function useUpdateOrganization(id: string) {
  const invalidate = useInvalidateOrg(id)
  return useMutation({
    meta: { errorToast: false },
    mutationFn: (patch: OrgProfilePatch) => patchOrganization(id, patch),
    onSuccess: invalidate,
  })
}

export function useUploadOrgAvatar(id: string) {
  const invalidate = useInvalidateOrg(id)
  return useMutation({
    meta: { errorToast: false },
    mutationFn: ({
      file,
      onProgress,
    }: {
      file: Blob
      onProgress?: (fraction: number) => void
    }) => putOrganizationAvatar(id, file, onProgress),
    onSuccess: invalidate,
  })
}

export function useRemoveOrgAvatar(id: string) {
  const invalidate = useInvalidateOrg(id)
  return useMutation({
    meta: { errorToast: false },
    mutationFn: () => deleteOrganizationAvatar(id),
    onSuccess: invalidate,
  })
}
