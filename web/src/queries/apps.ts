// Query-key factory and fetchers for the /apps resource, per
// frontend-plan.md section 2 and 4: no ad hoc key arrays inline in
// components, so invalidation after a deploy/rollback action can target
// the right keys precisely. Every route that needs app data imports these
// instead of writing its own fetch call or query key.

import {
  queryOptions,
  useMutation,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import type {
  AppDetail,
  AppListEntry,
  DeployStrategy,
  LogDrain,
} from '../types/appDetail'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const appKeys = {
  all: ['apps'] as const,
  list: () => [...appKeys.all, 'list'] as const,
  detail: (appId: string) => [...appKeys.all, 'detail', appId] as const,
}

// Fetches every app from the control plane API. GET /api/v1/apps
// (internal/api/apps.go's handleListApps) returns a bare array of
// appListResource (AppDetail plus a batched status summary, see
// AppListEntry's own doc comment), no cursor, so this is the one list
// fetcher, not a page-at-a-time one: there's nothing server-side yet to
// paginate against.
export async function fetchApps(): Promise<AppListEntry[]> {
  const res = await fetch('/api/v1/apps')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch apps failed: ${res.status}`),
    )
  }
  return (await res.json()) as AppListEntry[]
}

// Shared options object between the route loader's
// queryClient.ensureQueryData call and the component's useSuspenseQuery
// call, the same pattern appDetailQueryOptions below uses.
export function appListQueryOptions() {
  return queryOptions({
    queryKey: appKeys.list(),
    queryFn: fetchApps,
  })
}

// Fetches the full app resource for the detail route, GET
// /api/v1/apps/{name} (internal/api/apps.go's handleGetApp). Distinct
// from fetchAppPage above: that returns list-page summary rows, this
// returns everything the detail route needs to read and, on save,
// re-send in full (PUT is a full replace, see updateApp below).
export async function fetchApp(name: string): Promise<AppDetail> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(name)}`)
  if (res.status === 404) {
    throw new ApiError(404, `app not found: ${name}`)
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch app failed: ${res.status}`),
    )
  }
  return (await res.json()) as AppDetail
}

// Same shared-options pattern appListQueryOptions already established:
// one definition both the route loader's queryClient.ensureQueryData
// call and the component's useSuspenseQuery/useApp call share, so the
// AppDetail type flows through without either call site re-declaring it.
export function appDetailQueryOptions(name: string) {
  return queryOptions({
    queryKey: appKeys.detail(name),
    queryFn: () => fetchApp(name),
  })
}

// Thin wrapper the detail route's component uses instead of calling
// useSuspenseQuery(appDetailQueryOptions(name)) directly, mirroring the
// useApp(name) shape named in the frontend-plan style already
// established by appListQueryOptions/fetchAppPage above.
export function useApp(name: string) {
  return useSuspenseQuery(appDetailQueryOptions(name))
}

// PUT /api/v1/apps/{name} (internal/api/apps.go's handleUpdateApp), full
// replace semantics: the whole AppDetail is sent back on every call,
// there is no partial-patch endpoint. Callers (DomainEditor, EnvEditor)
// are responsible for spreading the current app and only swapping the
// field they actually edited.
export async function updateApp(app: AppDetail): Promise<AppDetail> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(app.name)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(app),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `update app failed: ${res.status}`),
    )
  }
  return (await res.json()) as AppDetail
}

// Shared mutation hook for both DomainEditor and EnvEditor: same
// endpoint, same full-replace payload shape, only the field that
// changed differs between call sites. On success, writes the server's
// response straight into the detail query's cache rather than
// invalidating and refetching, since the PUT response already is the
// new canonical AppDetail.
export function useUpdateApp(name: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: updateApp,
    onSuccess: (updated) => {
      queryClient.setQueryData(appKeys.detail(name), updated)
      // The list route renders each app's domains too (AppRow), so an
      // edit here must not leave a stale copy sitting in that cache.
      void queryClient.invalidateQueries({ queryKey: appKeys.list() })
    },
  })
}

// Request body for POST /api/v1/apps (internal/api/apps.go's
// handleCreateApp), deliberately narrower than the full appResource
// shape AppDetail models: env/resources/health are real fields the
// endpoint accepts, but CreateAppDialog only ever collects
// name/image/port (a manually-created app here is the "already have a
// built image" path in the app spec's model). domains is included since
// CreateAppFromGitFields collects one optional domain up front; the
// server still runs validateAppResource against whatever arrives, so
// leaving the rest undefined is safe, not a bypass.
// strategy/replicas are optional here on purpose: CreateAppFields only
// renders them as opt-in fields (leaving either blank omits it from the
// request body entirely), and the server's own validateAppResource
// (internal/api/apps.go) already treats an omitted/empty strategy and a
// zero replicas count as "use the store's default"
// (DefaultDeployStrategy/DefaultReplicas), the exact same fallthrough a
// service saved without these fields has always had.
// project_id is optional and, unlike node placement, sent directly in
// this same create request: handleCreateApp's own doc comment (internal/
// api/apps.go) explains why a brand-new app is safe to assign a project
// to at create time in a way an ordinary PUT update is not, so there's
// no CreateAppFields-side trailing useSetAppProject.mutate() call the
// way CreateAppFields still needs for node placement (setAppNode is a
// deliberately excluded-from-create mutation, project assignment isn't).
export interface CreateAppRequest {
  name: string
  image: string
  port: number
  strategy?: DeployStrategy
  replicas?: number
  project_id?: string
  domains?: string[]
}

// POST /api/v1/apps. Rejects a name that already exists with a 409
// (handleCreateApp's own existence check), which readErrorMessage below
// surfaces verbatim so CreateAppDialog can show the real server message
// on a conflict instead of a generic failure.
export async function createApp(req: CreateAppRequest): Promise<AppDetail> {
  const res = await fetch('/api/v1/apps', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `create app failed: ${res.status}`),
    )
  }
  return (await res.json()) as AppDetail
}

// Mirrors useUpdateApp's cache-write shape: the 201 response already is
// the canonical new AppDetail, so it's written straight into the detail
// query's cache (keyed by the name the server echoed back) rather than
// waiting on a refetch, so navigating straight to /apps/$name right
// after creation has data available immediately. The list query is
// invalidated rather than patched in place, same reasoning useUpdateApp
// documents above: AppRow renders fields this mutation never touches
// (domains), so a full refetch is simpler than hand-merging the new row
// in.
export function useCreateApp() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: createApp,
    onSuccess: (created) => {
      queryClient.setQueryData(appKeys.detail(created.name), created)
      void queryClient.invalidateQueries({ queryKey: appKeys.list() })
    },
  })
}

// PUT /api/v1/apps/{name}/node (internal/api/apps.go's
// handleSetAppNode), AbilityRoot-gated and separate from updateApp on
// purpose: see appResource's own NodeID field doc comment, this is the
// only way an app's placement actually changes. Empty nodeId moves the
// app back to this control plane's own local node, the store's
// established convention for "no explicit placement."
export async function setAppNode(
  name: string,
  nodeId: string,
): Promise<AppDetail> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(name)}/node`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ node_id: nodeId }),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `set app node failed: ${res.status}`),
    )
  }
  return (await res.json()) as AppDetail
}

export function useSetAppNode() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ name, nodeId }: { name: string; nodeId: string }) =>
      setAppNode(name, nodeId),
    onSuccess: (updated) => {
      queryClient.setQueryData(appKeys.detail(updated.name), updated)
      void queryClient.invalidateQueries({ queryKey: appKeys.list() })
    },
  })
}

// PUT /api/v1/apps/{name}/project (internal/api/apps.go's
// handleSetAppProject): the only way an existing app's project
// assignment changes, the project-kind counterpart to setAppNode above.
// Empty projectId moves the app back to "no project", the store's
// established convention for "not filed under any project."
export async function setAppProject(
  name: string,
  projectId: string,
): Promise<AppDetail> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(name)}/project`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ project_id: projectId }),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `set app project failed: ${res.status}`),
    )
  }
  return (await res.json()) as AppDetail
}

export function useSetAppProject() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ name, projectId }: { name: string; projectId: string }) =>
      setAppProject(name, projectId),
    onSuccess: (updated) => {
      queryClient.setQueryData(appKeys.detail(updated.name), updated)
      void queryClient.invalidateQueries({ queryKey: appKeys.list() })
    },
  })
}

// PUT /api/v1/apps/{name}/environment (internal/api/environments.go's
// handleSetAppEnvironment): the only way an existing app's environment
// tag changes, the same node_id/project_id shape setAppNode/setAppProject
// already establish. Empty environmentId clears the tag.
export async function setAppEnvironment(
  name: string,
  environmentId: string,
): Promise<AppDetail> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(name)}/environment`,
    {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ environment_id: environmentId }),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `set app environment failed: ${res.status}`),
    )
  }
  return (await res.json()) as AppDetail
}

export function useSetAppEnvironment() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({
      name,
      environmentId,
    }: {
      name: string
      environmentId: string
    }) => setAppEnvironment(name, environmentId),
    onSuccess: (updated) => {
      queryClient.setQueryData(appKeys.detail(updated.name), updated)
      void queryClient.invalidateQueries({ queryKey: appKeys.list() })
    },
  })
}

// PUT /api/v1/apps/{name}/storage (internal/api/apps_storage.go's
// handleSetAppStorage): attaches an already-connected backup target (the
// same S3-compatible bucket connection a database's scheduled backups can
// already point at, queries/backupTargets.ts) to this app as its
// object-storage credential source. Response is deliberately narrow
// (appStorageResource, just app_name/storage_target_id), not a full
// AppDetail, the same shape setDatabasePublicAccess's own response
// already establishes for its own dedicated endpoint, so the cache is
// patched directly below rather than replaced wholesale.
export interface AppStorageResource {
  app_name: string
  storage_target_id?: string
}

export async function setAppStorage(
  name: string,
  storageTargetId: string,
): Promise<AppStorageResource> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(name)}/storage`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ storage_target_id: storageTargetId }),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `set app storage failed: ${res.status}`),
    )
  }
  return (await res.json()) as AppStorageResource
}

export function useSetAppStorage() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({
      name,
      storageTargetId,
    }: {
      name: string
      storageTargetId: string
    }) => setAppStorage(name, storageTargetId),
    onSuccess: (result) => {
      queryClient.setQueryData(
        appKeys.detail(result.app_name),
        (existing: AppDetail | undefined) =>
          existing && {
            ...existing,
            storage_target_id: result.storage_target_id,
          },
      )
      void queryClient.invalidateQueries({
        queryKey: appKeys.detail(result.app_name),
      })
    },
  })
}

// DELETE /api/v1/apps/{name}/storage (handleClearAppStorage): detaches
// whatever backup target this app currently resolves object-storage
// credentials from. 204, no body, so the cache is patched directly from
// the name this mutation was called with, mirroring
// clearDatabasePublicAccess's own shape (queries/databases.ts).
export async function clearAppStorage(name: string): Promise<void> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(name)}/storage`, {
    method: 'DELETE',
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `clear app storage failed: ${res.status}`),
    )
  }
}

export function useClearAppStorage() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (name: string) => clearAppStorage(name),
    onSuccess: (_data, name) => {
      queryClient.setQueryData(
        appKeys.detail(name),
        (existing: AppDetail | undefined) =>
          existing && { ...existing, storage_target_id: undefined },
      )
      void queryClient.invalidateQueries({ queryKey: appKeys.detail(name) })
    },
  })
}

// PUT /api/v1/apps/{name}/log-drain (internal/api/apps_log_drain.go's
// handleSetAppLogDrain): forwards this app's container logs to an
// external HTTP or syslog sink, additive to the existing node-local log
// store. Response is deliberately narrow (LogDrainResource, just
// app_name plus the drain's own fields), the same shape
// AppStorageResource already establishes for its own dedicated
// endpoint.
export interface LogDrainResource {
  app_name: string
  type: LogDrain['type']
  target: string
  enabled: boolean
}

export async function setLogDrain(
  name: string,
  drain: LogDrain,
): Promise<LogDrainResource> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(name)}/log-drain`,
    {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(drain),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `set log drain failed: ${res.status}`),
    )
  }
  return (await res.json()) as LogDrainResource
}

export function useSetLogDrain() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ name, drain }: { name: string; drain: LogDrain }) =>
      setLogDrain(name, drain),
    onSuccess: (result) => {
      queryClient.setQueryData(
        appKeys.detail(result.app_name),
        (existing: AppDetail | undefined) =>
          existing && {
            ...existing,
            log_drain: {
              type: result.type,
              target: result.target,
              enabled: result.enabled,
            },
          },
      )
      void queryClient.invalidateQueries({
        queryKey: appKeys.detail(result.app_name),
      })
    },
  })
}

// DELETE /api/v1/apps/{name}/log-drain (handleClearAppLogDrain): stops
// forwarding this app's logs externally. 204, no body, so the cache is
// patched directly from the name this mutation was called with,
// mirroring clearAppStorage's own shape.
export async function clearLogDrain(name: string): Promise<void> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(name)}/log-drain`,
    {
      method: 'DELETE',
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `clear log drain failed: ${res.status}`),
    )
  }
}

export function useClearLogDrain() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (name: string) => clearLogDrain(name),
    onSuccess: (_data, name) => {
      queryClient.setQueryData(
        appKeys.detail(name),
        (existing: AppDetail | undefined) =>
          existing && { ...existing, log_drain: undefined },
      )
      void queryClient.invalidateQueries({ queryKey: appKeys.detail(name) })
    },
  })
}

// PUT /api/v1/apps/{name}/database (internal/api/apps_database.go's
// handleSetAppDatabase): attaches an already-created managed database to
// this app as a real, persisted connection-env-var source, the UI/CLI
// equivalent of app.yaml's own { from: "<database>.<field>" } syntax.
// Response is deliberately narrow (AppDatabaseResource), the same shape
// AppStorageResource already establishes for its own dedicated endpoint,
// so the cache is patched directly below rather than replaced wholesale.
export interface AppDatabaseResource {
  app_name?: string
  database_name: string
  env_var: string
  field: string
}

export async function setAppDatabase(
  name: string,
  req: { databaseName: string; envVar?: string; field?: string },
): Promise<AppDatabaseResource> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(name)}/database`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      database_name: req.databaseName,
      env_var: req.envVar,
      field: req.field,
    }),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `set app database failed: ${res.status}`),
    )
  }
  return (await res.json()) as AppDatabaseResource
}

export function useSetAppDatabase() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({
      name,
      databaseName,
      envVar,
      field,
    }: {
      name: string
      databaseName: string
      envVar?: string
      field?: string
    }) => setAppDatabase(name, { databaseName, envVar, field }),
    onSuccess: (result) => {
      queryClient.setQueryData(
        appKeys.detail(result.app_name ?? ''),
        (existing: AppDetail | undefined) =>
          existing && {
            ...existing,
            database_attachment: {
              database_name: result.database_name,
              env_var: result.env_var,
              field: result.field,
            },
          },
      )
      void queryClient.invalidateQueries({
        queryKey: appKeys.detail(result.app_name ?? ''),
      })
    },
  })
}

// DELETE /api/v1/apps/{name}/database (handleClearAppDatabase): detaches
// whatever database this app currently resolves its attachment env var
// from. 204, no body, mirroring clearAppStorage's own shape.
export async function clearAppDatabase(name: string): Promise<void> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(name)}/database`, {
    method: 'DELETE',
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `clear app database failed: ${res.status}`),
    )
  }
}

export function useClearAppDatabase() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (name: string) => clearAppDatabase(name),
    onSuccess: (_data, name) => {
      queryClient.setQueryData(
        appKeys.detail(name),
        (existing: AppDetail | undefined) =>
          existing && { ...existing, database_attachment: undefined },
      )
      void queryClient.invalidateQueries({ queryKey: appKeys.detail(name) })
    },
  })
}

// POST /api/v1/apps/{name}/restart (internal/api/apps.go's
// handleRestartApp): force a running container to be recreated with no
// image change, the only way to do that at all today (re-deploying the
// same image tag is otherwise a genuine reconciler no-op). No request
// body.
export async function restartApp(name: string): Promise<AppDetail> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(name)}/restart`, {
    method: 'POST',
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `restart app failed: ${res.status}`),
    )
  }
  return (await res.json()) as AppDetail
}

export function useRestartApp() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: restartApp,
    onSuccess: (updated) => {
      queryClient.setQueryData(appKeys.detail(updated.name), updated)
    },
  })
}

// POST /api/v1/apps/{name}/stop and .../start (internal/api/apps.go's
// handleStopApp/handleStartApp): sets/clears Suspended, distinct from
// delete (which removes desired state entirely). The reconciler, not
// this request, is what actually stops or starts containers. No request
// body.
async function stopApp(name: string): Promise<AppDetail> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(name)}/stop`, {
    method: 'POST',
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `stop app failed: ${res.status}`),
    )
  }
  return (await res.json()) as AppDetail
}

async function startApp(name: string): Promise<AppDetail> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(name)}/start`, {
    method: 'POST',
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `start app failed: ${res.status}`),
    )
  }
  return (await res.json()) as AppDetail
}

export function useStopApp() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: stopApp,
    onSuccess: (updated) => {
      queryClient.setQueryData(appKeys.detail(updated.name), updated)
      void queryClient.invalidateQueries({ queryKey: appKeys.list() })
    },
  })
}

export function useStartApp() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: startApp,
    onSuccess: (updated) => {
      queryClient.setQueryData(appKeys.detail(updated.name), updated)
      void queryClient.invalidateQueries({ queryKey: appKeys.list() })
    },
  })
}

// POST /api/v1/apps/{name}/clone (internal/api/apps_clone.go's
// handleCloneApp): duplicates name's desired state under newName. See
// that handler's own doc comment for exactly what does and doesn't
// carry over (image/port/env/secret-env-names/resources/health/
// strategy/replicas/project_id do, domains/secret-values/node_id
// deliberately don't). Rejects a newName that already exists with a 409,
// the same conflict shape createApp above already surfaces.
export async function cloneApp(
  name: string,
  newName: string,
): Promise<AppDetail> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(name)}/clone`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ new_name: newName }),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `clone app failed: ${res.status}`),
    )
  }
  return (await res.json()) as AppDetail
}

// Mirrors useCreateApp's cache-write shape exactly: the 201 response is
// the new clone's canonical AppDetail, written straight into its own
// detail query's cache so navigating to /apps/$name right after cloning
// has data immediately, and the list query is invalidated so /apps
// picks up the new row.
export function useCloneApp() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ name, newName }: { name: string; newName: string }) =>
      cloneApp(name, newName),
    onSuccess: (cloned) => {
      queryClient.setQueryData(appKeys.detail(cloned.name), cloned)
      void queryClient.invalidateQueries({ queryKey: appKeys.list() })
    },
  })
}

// DELETE /api/v1/apps/{name} (internal/api/apps.go's handleDeleteApp).
// Same known gap the backend doc comment names: removes desired state,
// does not itself stop or remove a running container, mirroring
// deleteDatabase's own reasoning above.
export async function deleteApp(name: string): Promise<void> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(name)}`, {
    method: 'DELETE',
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `delete app failed: ${res.status}`),
    )
  }
}

// Mirrors useDeleteDatabase's cache-invalidation shape exactly: the
// detail query is removed outright (the resource no longer exists) and
// the list query is invalidated so /apps reflects the deletion on next
// render.
export function useDeleteApp() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: deleteApp,
    onSuccess: (_data, name) => {
      queryClient.removeQueries({ queryKey: appKeys.detail(name) })
      void queryClient.invalidateQueries({ queryKey: appKeys.list() })
    },
  })
}
