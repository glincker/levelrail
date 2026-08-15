// Query-key factory and fetchers for the /projects resource
// (internal/api/projects.go), following the same shared-queryOptions
// pattern queries/backupTargets.ts and queries/nodes.ts already
// established: one options object per query, used by both a route
// loader's queryClient.ensureQueryData call and a component's
// useSuspenseQuery/hook call.
//
// A project is a lightweight, non-auth organizational label, not the
// start of a permission system (internal/api/projects.go's own package
// doc comment): create/list/get/delete only, nothing scoped to it.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import type { ProjectResource } from '../types/projectDetail'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { appKeys } from './apps'
import { databaseKeys } from './databases'

export const projectKeys = {
  all: ['projects'] as const,
  list: () => [...projectKeys.all, 'list'] as const,
  detail: (id: string) => [...projectKeys.all, 'detail', id] as const,
}

// GET /api/v1/projects (handleListProjects), a bare array, no cursor,
// the same "everything in one response" shape appListQueryOptions/
// nodeListQueryOptions already document for their own resources.
export async function fetchProjects(): Promise<ProjectResource[]> {
  const res = await fetch('/api/v1/projects')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch projects failed: ${res.status}`),
    )
  }
  return (await res.json()) as ProjectResource[]
}

export function projectListQueryOptions() {
  return queryOptions({
    queryKey: projectKeys.list(),
    queryFn: fetchProjects,
  })
}

// The Projects list route itself: always expected to succeed, matching
// useApp/useNodes' own ordinary-suspense shape.
export function useProjects() {
  return useSuspenseQuery(projectListQueryOptions())
}

// For call sites that show a project picker as an optional convenience
// inside another flow (the app/database create fields, MoveToProjectDialog):
// not suspense, no retry, mirroring useNodeListOptional's own doc
// comment exactly. A failure here degrades to "no projects to pick
// from" rather than blocking the surrounding form, since project
// assignment is itself always optional.
export function useProjectListOptional() {
  return useQuery({ ...projectListQueryOptions(), retry: false })
}

// GET /api/v1/projects/{id} (handleGetProject), for the project detail
// route.
export async function fetchProject(id: string): Promise<ProjectResource> {
  const res = await fetch(`/api/v1/projects/${encodeURIComponent(id)}`)
  if (res.status === 404) {
    throw new ApiError(404, `project not found: ${id}`)
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch project failed: ${res.status}`),
    )
  }
  return (await res.json()) as ProjectResource
}

export function projectDetailQueryOptions(id: string) {
  return queryOptions({
    queryKey: projectKeys.detail(id),
    queryFn: () => fetchProject(id),
  })
}

export function useProject(id: string) {
  return useSuspenseQuery(projectDetailQueryOptions(id))
}

// POST /api/v1/projects (handleCreateProject). No conflict case: unlike
// createApp/createDatabase, project names are deliberately not unique
// (migrations/0022_projects.sql's own comment), so there is no 409 to
// special-case here.
export async function createProject(name: string): Promise<ProjectResource> {
  const res = await fetch('/api/v1/projects', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `create project failed: ${res.status}`),
    )
  }
  return (await res.json()) as ProjectResource
}

export function useCreateProject() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: createProject,
    onSuccess: (created) => {
      queryClient.setQueryData(projectKeys.detail(created.id), created)
      void queryClient.invalidateQueries({ queryKey: projectKeys.list() })
    },
  })
}

// DELETE /api/v1/projects/{id} (handleDeleteProject). See that
// handler's own doc comment for the behavior this deliberately relies
// on: every app/database that belonged to this project is left running,
// simply project-less again (the backend's ON DELETE SET NULL foreign
// key), never deleted. Both the apps and databases list/detail caches
// are invalidated here for exactly that reason: their own project_id
// field just changed for every row that belonged to this project, the
// same cross-resource invalidation reasoning useDrainNode already
// applies to its own multi-resource side effect.
export async function deleteProject(id: string): Promise<void> {
  const res = await fetch(`/api/v1/projects/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `delete project failed: ${res.status}`),
    )
  }
}

export function useDeleteProject() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: deleteProject,
    onSuccess: (_data, id) => {
      queryClient.removeQueries({ queryKey: projectKeys.detail(id) })
      void queryClient.invalidateQueries({ queryKey: projectKeys.list() })
      void queryClient.invalidateQueries({ queryKey: appKeys.list() })
      void queryClient.invalidateQueries({ queryKey: databaseKeys.list() })
    },
  })
}
