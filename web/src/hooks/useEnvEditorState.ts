import { useCallback, useEffect, useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import type { AppDetail } from '../types/appDetail'
import { appKeys } from '../queries/apps'
import {
  applyEnv,
  EnvVersionConflictError,
  type EnvApplyBody,
} from '../queries/envApply'
import { projectDetailQueryOptions } from '../queries/projects'
import { organizationEnvQueryOptions } from '../queries/organizationEnv'
import { projectEnvQueryOptions } from '../queries/projectEnv'
import { environmentEnvQueryOptions } from '../queries/environmentEnv'
import {
  secretKeys as secretQueryKeys,
  useSecretKeys,
} from '../queries/secrets'
import {
  computeInheritedEnv,
  inheritedOnlyEnv,
  type EnvTier,
} from '../lib/envProvenance'
import {
  buildRows,
  isBuildTimeKey,
  MASKED_VALUE,
  buildSavePlan,
  computeChanges,
  type EnvRow,
} from '../lib/envRows'

export interface EnvConflict {
  currentVersion: number
  keys: string[]
}

const draftStoreKey = (app: string) => `env-draft:${app}`

function readStoredDraft(app: string): EnvRow[] | null {
  try {
    const raw = sessionStorage.getItem(draftStoreKey(app))
    return raw ? (JSON.parse(raw) as EnvRow[]) : null
  } catch {
    return null
  }
}

// Secret values never reach sessionStorage, only their names.
function writeStoredDraft(app: string, rows: EnvRow[] | null) {
  try {
    if (rows === null) sessionStorage.removeItem(draftStoreKey(app))
    else
      sessionStorage.setItem(
        draftStoreKey(app),
        JSON.stringify(
          rows.map((r) => (r.type === 'secret' ? { ...r, value: '' } : r)),
        ),
      )
  } catch {
    // Blocked storage: the in-memory draft still holds.
  }
}

function useInheritedLayers(app: AppDetail) {
  const projectId = app.project_id ?? ''
  const environmentId = app.environment_id ?? ''
  const project = useQuery({
    ...projectDetailQueryOptions(projectId),
    enabled: projectId !== '',
  })
  const orgId = project.data?.org_id ?? ''
  const org = useQuery({
    ...organizationEnvQueryOptions(orgId),
    enabled: orgId !== '',
  })
  const proj = useQuery({
    ...projectEnvQueryOptions(projectId),
    enabled: projectId !== '',
  })
  const env = useQuery({
    ...environmentEnvQueryOptions(environmentId),
    enabled: environmentId !== '',
  })
  return useMemo(
    () => ({
      organization: org.data,
      project: proj.data,
      environment: env.data,
    }),
    [org.data, proj.data, env.data],
  )
}

// Owns the editor's draft rows, built from the server state and saved back
// through the right write for each type (plain env, secret, vault ref).
export function useEnvEditorState(app: AppDetail) {
  const queryClient = useQueryClient()
  const secretsQuery = useSecretKeys(app.name)
  const layers = useInheritedLayers(app)
  const secretNames = useMemo(
    () => [
      ...(app.secret_env ?? []),
      ...(secretsQuery.data ?? []).map((s) => s.key),
    ],
    [app.secret_env, secretsQuery.data],
  )
  const baseRows = useMemo(
    () =>
      buildRows({
        env: app.env,
        envMeta: app.env_meta,
        secretKeys: secretNames,
        vaultEnv: app.vault_env,
        inherited: inheritedOnlyEnv(layers, {}),
      }),
    [app.env, app.env_meta, app.vault_env, secretNames, layers],
  )
  const inheritedTiers = useMemo(() => {
    const out: Record<string, EnvTier> = {}
    for (const [key, info] of Object.entries(computeInheritedEnv(layers)))
      out[key] = info.tier
    return out
  }, [layers])

  const [draft, setDraft] = useState<EnvRow[] | null>(() =>
    readStoredDraft(app.name),
  )
  const [draftVersion, setDraftVersion] = useState<number | null>(null)
  const [conflict, setConflict] = useState<EnvConflict | null>(null)
  const [replaceLocked, setReplaceLocked] = useState<Set<string>>(new Set())
  const serverVersion = app.env_version ?? 0
  useEffect(() => {
    writeStoredDraft(app.name, draft)
  }, [app.name, draft])
  const rows = draft ?? baseRows
  const changes = useMemo(() => computeChanges(rows), [rows])
  const dirty = draft !== null && changes.length > 0
  const lockedKeys = useMemo(
    () =>
      new Set(
        (secretsQuery.data ?? []).filter((x) => x.locked).map((x) => x.key),
      ),
    [secretsQuery.data],
  )
  const lockedPending = useMemo(
    () =>
      changes
        .filter((c) => c.type === 'secret' && lockedKeys.has(c.key))
        .map((c) => c.key),
    [changes, lockedKeys],
  )
  const lockedBlocked = lockedPending.some((k) => !replaceLocked.has(k))
  const remoteChanged =
    dirty && draftVersion !== null && serverVersion !== draftVersion
  const toggleReplaceLocked = useCallback((key: string, on: boolean) => {
    setReplaceLocked((prev) => {
      const next = new Set(prev)
      if (on) next.add(key)
      else next.delete(key)
      return next
    })
  }, [])
  const markDraft = useCallback(() => {
    setDraftVersion((v) => v ?? serverVersion)
  }, [serverVersion])

  const updateRow = useCallback(
    (row: EnvRow) => {
      markDraft()
      setDraft((prev) =>
        (prev ?? baseRows).map((r) => (r.id === row.id ? row : r)),
      )
    },
    [baseRows, markDraft],
  )
  const addRow = useCallback(
    (row: EnvRow) => {
      markDraft()
      setDraft((prev) =>
        [...(prev ?? baseRows), row].sort((a, b) => a.key.localeCompare(b.key)),
      )
    },
    [baseRows, markDraft],
  )
  const replaceRows = useCallback(
    (next: EnvRow[]) => {
      markDraft()
      setDraft(next)
    },
    [markDraft],
  )
  const discard = useCallback(() => {
    setDraft(null)
    setDraftVersion(null)
    setConflict(null)
    setReplaceLocked(new Set())
  }, [])

  // One apply request with the version read at load; the draft survives any failure.
  const save = useCallback(
    async (versionOverride?: number): Promise<string[]> => {
      const plan = buildSavePlan(rows)
      const body: EnvApplyBody = {
        expected_version: versionOverride ?? draftVersion ?? serverVersion,
        overwrite_locked: lockedPending.length > 0,
      }
      const live = app.env ?? {}
      const set = Object.fromEntries(
        Object.entries(plan.env).filter(
          ([k, v]) => v !== MASKED_VALUE && live[k] !== v,
        ),
      )
      const unset = rows
        .filter((r) => r.removed && r.original?.type === 'plain')
        .map((r) => r.key)
      if (Object.keys(set).length > 0) body.set = set
      if (unset.length > 0) body.unset = unset
      if (plan.secretsToSet.length > 0)
        body.secret_set = Object.fromEntries(
          plan.secretsToSet.map((x) => [x.key, x.value]),
        )
      if (plan.secretsToDelete.length > 0)
        body.secret_delete = plan.secretsToDelete
      if (plan.vaultToSet.length > 0)
        body.vault_set = Object.fromEntries(
          plan.vaultToSet.map((v) => [
            v.key,
            { path: v.ref.path, key: v.ref.key },
          ]),
        )
      if (plan.vaultToClear.length > 0) body.vault_clear = plan.vaultToClear
      try {
        const saved = await applyEnv(app.name, body)
        await Promise.all([
          queryClient.invalidateQueries({ queryKey: appKeys.detail(app.name) }),
          queryClient.invalidateQueries({
            queryKey: secretQueryKeys.list(app.name),
          }),
        ])
        setDraft(null)
        setDraftVersion(null)
        setConflict(null)
        setReplaceLocked(new Set())
        return saved.plan.map((c) => c.key).filter(isBuildTimeKey)
      } catch (error) {
        if (error instanceof EnvVersionConflictError)
          setConflict({
            currentVersion: error.currentVersion,
            keys: error.planKeys,
          })
        throw error
      }
    },
    [rows, app, queryClient, draftVersion, serverVersion, lockedPending],
  )

  // Keep mine for the chosen keys, take theirs for the rest, then save on the live version.
  const resolveConflict = useCallback(
    async (keepKeys: string[]): Promise<string[]> => {
      if (!conflict) return []
      const theirs = new Map(baseRows.map((r) => [r.key, r]))
      const merged = rows.flatMap((r) => {
        if (keepKeys.includes(r.key) || !conflict.keys.includes(r.key))
          return [r]
        const t = theirs.get(r.key)
        return t ? [t] : []
      })
      setDraft(merged)
      setDraftVersion(conflict.currentVersion)
      return save(conflict.currentVersion)
    },
    [conflict, rows, baseRows, save],
  )
  const dismissConflict = useCallback(() => {
    setConflict(null)
  }, [])

  return {
    rows,
    changes,
    dirty,
    inheritedTiers,
    secretsLoading: secretsQuery.isLoading,
    updateRow,
    addRow,
    replaceRows,
    discard,
    save,
    conflict,
    resolveConflict,
    dismissConflict,
    lockedPending,
    lockedBlocked,
    replaceLocked,
    toggleReplaceLocked,
    remoteChanged,
  }
}
