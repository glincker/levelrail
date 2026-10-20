import { useCallback, useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import type { AppDetail } from '../types/appDetail'
import { appKeys, updateApp } from '../queries/apps'
import { projectDetailQueryOptions } from '../queries/projects'
import { organizationEnvQueryOptions } from '../queries/organizationEnv'
import { projectEnvQueryOptions } from '../queries/projectEnv'
import { environmentEnvQueryOptions } from '../queries/environmentEnv'
import {
  secretKeys as secretQueryKeys,
  setSecret,
  useSecretKeys,
} from '../queries/secrets'
import { clearAppVaultEnv, setAppVaultEnv } from '../queries/appVaultEnv'
import { deleteSecretValue } from '../queries/envProtection'
import {
  computeInheritedEnv,
  inheritedOnlyEnv,
  type EnvTier,
} from '../lib/envProvenance'
import {
  buildRows,
  buildSavePlan,
  computeChanges,
  type EnvRow,
} from '../lib/envRows'

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

  const [draft, setDraft] = useState<EnvRow[] | null>(null)
  const rows = draft ?? baseRows
  const changes = useMemo(() => computeChanges(rows), [rows])
  const dirty = draft !== null && changes.length > 0

  const updateRow = useCallback(
    (row: EnvRow) => {
      setDraft((prev) =>
        (prev ?? baseRows).map((r) => (r.id === row.id ? row : r)),
      )
    },
    [baseRows],
  )
  const addRow = useCallback(
    (row: EnvRow) => {
      setDraft((prev) =>
        [...(prev ?? baseRows), row].sort((a, b) => a.key.localeCompare(b.key)),
      )
    },
    [baseRows],
  )
  const replaceRows = useCallback((next: EnvRow[]) => {
    setDraft(next)
  }, [])
  const discard = useCallback(() => {
    setDraft(null)
  }, [])

  const save = useCallback(async () => {
    const plan = buildSavePlan(rows)
    if (plan.envChanged) {
      const saved = await updateApp({ ...app, env: plan.env })
      queryClient.setQueryData(
        appKeys.detail(app.name),
        (prev: AppDetail | undefined) =>
          prev
            ? {
                ...prev,
                env: saved.env,
                env_meta: saved.env_meta,
                env_dirty: saved.env_dirty,
              }
            : saved,
      )
    }
    for (const s of plan.secretsToSet) {
      await setSecret(app.name, {
        key: s.key,
        value: s.value,
        overwriteLocked: true,
      })
    }
    for (const key of plan.secretsToDelete)
      await deleteSecretValue(app.name, key)
    for (const v of plan.vaultToSet) {
      await setAppVaultEnv(app.name, {
        key: v.key,
        path: v.ref.path,
        vaultKey: v.ref.key,
      })
    }
    for (const key of plan.vaultToClear) await clearAppVaultEnv(app.name, key)
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: appKeys.detail(app.name) }),
      queryClient.invalidateQueries({
        queryKey: secretQueryKeys.list(app.name),
      }),
    ])
    setDraft(null)
  }, [rows, app, queryClient])

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
  }
}
