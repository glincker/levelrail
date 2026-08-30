// Query-key factory and fetchers for the /databases resource, mirroring
// queries/apps.ts's shape exactly: no ad hoc key arrays inline in
// components, so every route that needs database data imports these
// instead of writing its own fetch call or query key.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import type {
  DatabaseListEntry,
  DatabaseResource,
  ServiceResources,
} from '../types/databaseDetail'
import type { ReconcileCondition } from '../types/deploy'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const databaseKeys = {
  all: ['databases'] as const,
  list: () => [...databaseKeys.all, 'list'] as const,
  detail: (name: string) => [...databaseKeys.all, 'detail', name] as const,
  status: (name: string) => [...databaseKeys.detail(name), 'status'] as const,
}

// Fetches every database from the control plane API. GET
// /api/v1/databases (internal/api/databases.go's handleListDatabases)
// returns databaseListResource, databaseResource plus a batched status
// summary, no cursor, matching fetchApps's own reasoning.
export async function fetchDatabases(): Promise<DatabaseListEntry[]> {
  const res = await fetch('/api/v1/databases')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch databases failed: ${res.status}`),
    )
  }
  return (await res.json()) as DatabaseListEntry[]
}

// Shared options object between the route loader's
// queryClient.ensureQueryData call and the component's useSuspenseQuery
// call, the same pattern databaseDetailQueryOptions below uses.
export function databaseListQueryOptions() {
  return queryOptions({
    queryKey: databaseKeys.list(),
    queryFn: fetchDatabases,
  })
}

// useDatabases is databaseListQueryOptions' plain (non-suspense) form,
// the same "component decides its own loading UI, no route-level
// Suspense boundary" shape useBackupTargetsOptional (queries/
// backupTargets.ts) already establishes for a picker component. Unlike
// that hook, retry isn't disabled: GET /api/v1/databases needs no
// optional server config the way backup targets' 501 case does, so an
// ordinary transient failure is worth TanStack Query's default retry.
export function useDatabases() {
  return useQuery(databaseListQueryOptions())
}

// Fetches a single database resource for the detail route, GET
// /api/v1/databases/{name} (internal/api/databases.go's
// handleGetDatabase).
export async function fetchDatabase(name: string): Promise<DatabaseResource> {
  const res = await fetch(`/api/v1/databases/${encodeURIComponent(name)}`)
  if (res.status === 404) {
    throw new ApiError(404, `database not found: ${name}`)
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch database failed: ${res.status}`),
    )
  }
  return (await res.json()) as DatabaseResource
}

export function databaseDetailQueryOptions(name: string) {
  return queryOptions({
    queryKey: databaseKeys.detail(name),
    queryFn: () => fetchDatabase(name),
  })
}

// Thin wrapper the detail route's component uses instead of calling
// useSuspenseQuery(databaseDetailQueryOptions(name)) directly, mirroring
// useApp(name)'s shape.
export function useDatabase(name: string) {
  return useSuspenseQuery(databaseDetailQueryOptions(name))
}

// GET /api/v1/databases/{name}/status (internal/api/databases.go's
// handleDatabaseStatus): the same ReconcileCondition[] shape
// deployStatusQueryOptions returns for apps, per the task's frozen
// contract, reused as-is rather than modeled with a new type. This is
// the only place a caller can see that a postgres database exists but
// isn't actually running yet, see databaseResource's own doc comment on
// the backend.
export async function fetchDatabaseStatus(
  name: string,
): Promise<ReconcileCondition[]> {
  const res = await fetch(
    `/api/v1/databases/${encodeURIComponent(name)}/status`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch database status failed: ${res.status}`,
      ),
    )
  }
  const body = (await res.json()) as ReconcileCondition[] | null
  return body ?? []
}

export function databaseStatusQueryOptions(name: string) {
  return queryOptions({
    queryKey: databaseKeys.status(name),
    queryFn: () => fetchDatabaseStatus(name),
  })
}

export function useDatabaseStatus(name: string) {
  return useSuspenseQuery(databaseStatusQueryOptions(name))
}

// Request body for POST /api/v1/databases (internal/api/databases.go's
// handleCreateDatabase): name/engine/version are the entire
// databaseResource shape minus the response-only node_id, so this
// request type and DatabaseResource are almost the same thing, kept
// separate anyway for the same reason CreateAppRequest is kept separate
// from AppDetail: the request is what a form collects, the response is
// what the server may additionally attach.
// project_id is optional and sent directly in this same create request,
// the database-kind counterpart to CreateAppRequest's own project_id
// field: see that type's own doc comment (queries/apps.ts) for why
// that's safe at create time in a way it isn't through an ordinary
// update.
export interface CreateDatabaseRequest {
  name: string
  engine: string
  version: string
  project_id?: string
}

// POST /api/v1/databases. Rejects a name that already exists with a 409
// (handleCreateDatabase's own existence check), which readErrorMessage
// below surfaces verbatim so CreateDatabaseDialog can show the real
// server message on a conflict instead of a generic failure.
export async function createDatabase(
  req: CreateDatabaseRequest,
): Promise<DatabaseResource> {
  const res = await fetch('/api/v1/databases', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `create database failed: ${res.status}`),
    )
  }
  return (await res.json()) as DatabaseResource
}

// Mirrors useCreateApp's cache-write shape: the 201 response already is
// the canonical new DatabaseResource, so it's written straight into the
// detail query's cache (keyed by the name the server echoed back)
// rather than waiting on a refetch, so navigating straight to
// /databases/$name right after creation has data available immediately.
// The list query is invalidated rather than patched in place for the
// same simplicity reasoning useCreateApp gives.
export function useCreateDatabase() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: createDatabase,
    onSuccess: (created) => {
      queryClient.setQueryData(databaseKeys.detail(created.name), created)
      void queryClient.invalidateQueries({ queryKey: databaseKeys.list() })
    },
  })
}

// DELETE /api/v1/databases/{name} (internal/api/databases.go's
// handleDeleteDatabase). Same known gap the backend doc comment names
// for handleDeleteApp: removes desired state, does not itself stop or
// remove a running container.
export async function deleteDatabase(name: string): Promise<void> {
  const res = await fetch(`/api/v1/databases/${encodeURIComponent(name)}`, {
    method: 'DELETE',
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `delete database failed: ${res.status}`),
    )
  }
}

export function useDeleteDatabase() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: deleteDatabase,
    onSuccess: (_data, name) => {
      queryClient.removeQueries({ queryKey: databaseKeys.detail(name) })
      void queryClient.invalidateQueries({ queryKey: databaseKeys.list() })
    },
  })
}

// PUT /api/v1/databases/{name}/node (internal/api/databases.go's
// handleSetDatabaseNode), the database counterpart to queries/apps.ts's
// setAppNode/useSetAppNode: same AbilityRoot gating, same empty-string-
// means-local-node convention.
export async function setDatabaseNode(
  name: string,
  nodeId: string,
): Promise<DatabaseResource> {
  const res = await fetch(
    `/api/v1/databases/${encodeURIComponent(name)}/node`,
    {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ node_id: nodeId }),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `set database node failed: ${res.status}`),
    )
  }
  return (await res.json()) as DatabaseResource
}

export function useSetDatabaseNode() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ name, nodeId }: { name: string; nodeId: string }) =>
      setDatabaseNode(name, nodeId),
    onSuccess: (updated) => {
      queryClient.setQueryData(databaseKeys.detail(updated.name), updated)
      void queryClient.invalidateQueries({ queryKey: databaseKeys.list() })
    },
  })
}

// PUT /api/v1/databases/{name}/project (internal/api/databases.go's
// handleSetDatabaseProject): the only way an existing database's
// project assignment changes, the project-kind counterpart to
// setDatabaseNode above.
export async function setDatabaseProject(
  name: string,
  projectId: string,
): Promise<DatabaseResource> {
  const res = await fetch(
    `/api/v1/databases/${encodeURIComponent(name)}/project`,
    {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ project_id: projectId }),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `set database project failed: ${res.status}`),
    )
  }
  return (await res.json()) as DatabaseResource
}

export function useSetDatabaseProject() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ name, projectId }: { name: string; projectId: string }) =>
      setDatabaseProject(name, projectId),
    onSuccess: (updated) => {
      queryClient.setQueryData(databaseKeys.detail(updated.name), updated)
      void queryClient.invalidateQueries({ queryKey: databaseKeys.list() })
    },
  })
}

// Response shape for PUT/DELETE /api/v1/databases/{name}/public-access
// (internal/api/database_public_access.go's databasePublicAccessResource):
// only the three fields that endpoint actually changes, not the full
// DatabaseResource, mirroring that handler's own "deliberately its own
// small type" reasoning.
export interface DatabasePublicAccessResource {
  database_name: string
  publicly_accessible: boolean
  public_port?: number
}

// PUT /api/v1/databases/{name}/public-access. port omitted or 0 means
// "auto-assign the next free port" (store.SetDatabasePublicAccess's own
// default); a specific port is validated server-side against the
// 1024-65535 range and the control plane's own reserved ports before
// being persisted.
export async function setDatabasePublicAccess(
  name: string,
  port?: number,
): Promise<DatabasePublicAccessResource> {
  const res = await fetch(
    `/api/v1/databases/${encodeURIComponent(name)}/public-access`,
    {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(port ? { port } : {}),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `set database public access failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as DatabasePublicAccessResource
}

// On success, patches the cached DatabaseResource directly (the same
// "don't wait on a refetch" reasoning useSetDatabaseNode already
// follows) rather than replacing it wholesale: the PUT response only
// carries the three public-access fields, not the full resource shape.
export function useSetDatabasePublicAccess() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ name, port }: { name: string; port?: number }) =>
      setDatabasePublicAccess(name, port),
    onSuccess: (result) => {
      queryClient.setQueryData(
        databaseKeys.detail(result.database_name),
        (existing: DatabaseResource | undefined) =>
          existing && {
            ...existing,
            publicly_accessible: result.publicly_accessible,
            public_port: result.public_port,
          },
      )
      void queryClient.invalidateQueries({
        queryKey: databaseKeys.detail(result.database_name),
      })
    },
  })
}

// DELETE /api/v1/databases/{name}/public-access
// (handleClearDatabasePublicAccess): returns the database to
// internal-network-only. 204, no body, so the cache is patched directly
// from the name this mutation was called with rather than a server
// response.
export async function clearDatabasePublicAccess(name: string): Promise<void> {
  const res = await fetch(
    `/api/v1/databases/${encodeURIComponent(name)}/public-access`,
    { method: 'DELETE' },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `clear database public access failed: ${res.status}`,
      ),
    )
  }
}

export function useClearDatabasePublicAccess() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (name: string) => clearDatabasePublicAccess(name),
    onSuccess: (_data, name) => {
      queryClient.setQueryData(
        databaseKeys.detail(name),
        (existing: DatabaseResource | undefined) =>
          existing && {
            ...existing,
            publicly_accessible: false,
            public_port: undefined,
          },
      )
      void queryClient.invalidateQueries({
        queryKey: databaseKeys.detail(name),
      })
    },
  })
}

// PUT /api/v1/databases/{name}/resources
// (internal/api/databases.go's handleSetDatabaseResources): the database
// counterpart to queries/apps.ts's useUpdateApp for its own `resources`
// field, but as a dedicated route rather than a general replace, since
// databases have no PUT /databases/{name} (see handleSetDatabaseResources'
// own doc comment on the backend). resources: null clears a
// previously-set limit, matching setDatabaseBackupSchedule's own
// clear-by-explicit-empty-value convention.
export async function setDatabaseResources(
  name: string,
  resources: ServiceResources | null,
): Promise<DatabaseResource> {
  const res = await fetch(
    `/api/v1/databases/${encodeURIComponent(name)}/resources`,
    {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ resources }),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `set database resources failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as DatabaseResource
}

export function useSetDatabaseResources() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({
      name,
      resources,
    }: {
      name: string
      resources: ServiceResources | null
    }) => setDatabaseResources(name, resources),
    onSuccess: (updated) => {
      queryClient.setQueryData(databaseKeys.detail(updated.name), updated)
      void queryClient.invalidateQueries({ queryKey: databaseKeys.list() })
    },
  })
}

// BackupScheduleResource mirrors internal/api's backupScheduleResource
// (internal/api/backups.go): PUT/DELETE only ever touch these three
// fields, never the full database resource.
export interface BackupScheduleResource {
  database_name: string
  target_id?: string
  schedule?: string
  retain?: number
  retain_days?: number
}

export interface SetDatabaseBackupScheduleRequest {
  name: string
  targetId: string
  schedule: string
  retain: number
  retainDays: number
}

// PUT /api/v1/databases/{name}/backup-schedule
// (handleSetBackupSchedule). 400 means an invalid cron expression or a
// missing target_id; 404 means the database or target itself is gone.
export async function setDatabaseBackupSchedule({
  name,
  targetId,
  schedule,
  retain,
  retainDays,
}: SetDatabaseBackupScheduleRequest): Promise<BackupScheduleResource> {
  const res = await fetch(
    `/api/v1/databases/${encodeURIComponent(name)}/backup-schedule`,
    {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        target_id: targetId,
        schedule,
        retain,
        retain_days: retainDays,
      }),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `set backup schedule failed: ${res.status}`),
    )
  }
  return (await res.json()) as BackupScheduleResource
}

// Patches the cached DatabaseResource directly, the same "PUT response
// only carries a few fields, not the full resource" reasoning
// useSetDatabasePublicAccess already follows.
export function useSetDatabaseBackupSchedule() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: setDatabaseBackupSchedule,
    onSuccess: (result) => {
      queryClient.setQueryData(
        databaseKeys.detail(result.database_name),
        (existing: DatabaseResource | undefined) =>
          existing && {
            ...existing,
            backup_target_id: result.target_id,
            backup_schedule: result.schedule,
            backup_retain: result.retain,
            backup_retain_days: result.retain_days,
          },
      )
      void queryClient.invalidateQueries({
        queryKey: databaseKeys.detail(result.database_name),
      })
    },
  })
}

// DELETE /api/v1/databases/{name}/backup-schedule
// (handleClearBackupSchedule): 204, no body, so the cache is patched
// directly from the name this mutation was called with, mirroring
// useClearDatabasePublicAccess exactly.
export async function clearDatabaseBackupSchedule(name: string): Promise<void> {
  const res = await fetch(
    `/api/v1/databases/${encodeURIComponent(name)}/backup-schedule`,
    { method: 'DELETE' },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `clear backup schedule failed: ${res.status}`,
      ),
    )
  }
}

export function useClearDatabaseBackupSchedule() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (name: string) => clearDatabaseBackupSchedule(name),
    onSuccess: (_data, name) => {
      queryClient.setQueryData(
        databaseKeys.detail(name),
        (existing: DatabaseResource | undefined) =>
          existing && {
            ...existing,
            backup_target_id: undefined,
            backup_schedule: undefined,
            backup_retain: undefined,
            backup_retain_days: undefined,
          },
      )
      void queryClient.invalidateQueries({
        queryKey: databaseKeys.detail(name),
      })
    },
  })
}
