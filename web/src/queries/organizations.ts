// Query-key factory and fetchers for the /organizations resource
// (internal/api/organizations.go), following queries/projects.ts' own
// shared-queryOptions pattern exactly. An organization groups projects,
// nothing more: create/list/get/delete plus the project-assignment
// endpoint, no member list or ability of its own.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import type { OrganizationResource } from '../types/organization'
import type { ProjectResource } from '../types/projectDetail'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { projectKeys } from './projects'

export const organizationKeys = {
  all: ['organizations'] as const,
  list: () => [...organizationKeys.all, 'list'] as const,
  detail: (id: string) => [...organizationKeys.all, 'detail', id] as const,
}

export async function fetchOrganizations(): Promise<OrganizationResource[]> {
  const res = await fetch('/api/v1/organizations')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch organizations failed: ${res.status}`),
    )
  }
  return (await res.json()) as OrganizationResource[]
}

export function organizationListQueryOptions() {
  return queryOptions({
    queryKey: organizationKeys.list(),
    queryFn: fetchOrganizations,
  })
}

export function useOrganizations() {
  return useSuspenseQuery(organizationListQueryOptions())
}

// For an optional picker inside another flow (MoveToOrganizationDialog):
// mirrors useProjectListOptional's own doc comment exactly. A failure
// here degrades to "no organizations to pick from" rather than blocking
// the surrounding form.
export function useOrganizationListOptional() {
  return useQuery({ ...organizationListQueryOptions(), retry: false })
}

// GET /api/v1/organizations/{id} (handleGetOrganization), for the
// organization detail route.
export async function fetchOrganization(
  id: string,
): Promise<OrganizationResource> {
  const res = await fetch(`/api/v1/organizations/${encodeURIComponent(id)}`)
  if (res.status === 404) {
    throw new ApiError(404, `organization not found: ${id}`)
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch organization failed: ${res.status}`),
    )
  }
  return (await res.json()) as OrganizationResource
}

export function organizationDetailQueryOptions(id: string) {
  return queryOptions({
    queryKey: organizationKeys.detail(id),
    queryFn: () => fetchOrganization(id),
  })
}

export function useOrganization(id: string) {
  return useSuspenseQuery(organizationDetailQueryOptions(id))
}

export async function createOrganization(
  name: string,
): Promise<OrganizationResource> {
  const res = await fetch('/api/v1/organizations', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `create organization failed: ${res.status}`),
    )
  }
  return (await res.json()) as OrganizationResource
}

export function useCreateOrganization() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: createOrganization,
    onSuccess: (created) => {
      queryClient.setQueryData(organizationKeys.detail(created.id), created)
      void queryClient.invalidateQueries({ queryKey: organizationKeys.list() })
    },
  })
}

// DELETE /api/v1/organizations/{id} (handleDeleteOrganization): every
// project filed under it survives, simply organization-less again (the
// backend's ON DELETE SET NULL foreign key). The project list/detail
// caches are invalidated for exactly that reason, the same cross-resource
// reasoning useDeleteProject's own doc comment gives for apps/databases.
export async function deleteOrganization(id: string): Promise<void> {
  const res = await fetch(`/api/v1/organizations/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `delete organization failed: ${res.status}`),
    )
  }
}

export function useDeleteOrganization() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: deleteOrganization,
    onSuccess: (_data, id) => {
      queryClient.removeQueries({ queryKey: organizationKeys.detail(id) })
      void queryClient.invalidateQueries({ queryKey: organizationKeys.list() })
      void queryClient.invalidateQueries({ queryKey: projectKeys.list() })
    },
  })
}

// PUT /api/v1/projects/{id}/organization (handleSetProjectOrganization):
// the only way an existing project's organization assignment changes.
// Empty orgId moves the project back to "no organization", mirroring
// setAppProject's own empty-string-clears convention.
export async function setProjectOrganization(
  projectId: string,
  orgId: string,
): Promise<ProjectResource> {
  const res = await fetch(
    `/api/v1/projects/${encodeURIComponent(projectId)}/organization`,
    {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ org_id: orgId }),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `set project organization failed: ${res.status}`),
    )
  }
  return (await res.json()) as ProjectResource
}

export function useSetProjectOrganization() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ projectId, orgId }: { projectId: string; orgId: string }) =>
      setProjectOrganization(projectId, orgId),
    onSuccess: (updated) => {
      queryClient.setQueryData(projectKeys.detail(updated.id), updated)
      void queryClient.invalidateQueries({ queryKey: projectKeys.list() })
    },
  })
}
