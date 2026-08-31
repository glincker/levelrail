import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import type { AppDetail } from '../types/appDetail'
import { useUpdateApp } from '../queries/apps'
import { useRestartRequiredToast } from '../hooks/useRestartRequiredToast'
import { projectDetailQueryOptions } from '../queries/projects'
import { organizationEnvQueryOptions } from '../queries/organizationEnv'
import { projectEnvQueryOptions } from '../queries/projectEnv'
import { environmentEnvQueryOptions } from '../queries/environmentEnv'
import { computeInheritedEnv, inheritedOnlyEnv } from '../lib/envProvenance'
import { EnvVarsForm } from './EnvVarsForm'
import { EnvActivityPanel } from './EnvActivityPanel'
import { Badge } from '@/components/ui/badge'

// Deliberately plain string values only: internal/deploy.requireNoUnresolvedEnv
// still rejects app.yaml's `{ secret: true }` / `{ from: ... }` env
// references outright, and store.DesiredService.Env is a flat
// map[string]string with no room to represent a secret reference even
// if this editor tried.
export function EnvEditor({ app }: { app: AppDetail }) {
  const updateApp = useUpdateApp(app.name)
  const notifyRestartRequired = useRestartRequiredToast()

  const projectId = app.project_id
  const environmentId = app.environment_id

  const projectQuery = useQuery({
    ...projectDetailQueryOptions(projectId ?? ''),
    enabled: Boolean(projectId),
  })
  const orgId = projectQuery.data?.org_id

  const orgEnvQuery = useQuery({
    ...organizationEnvQueryOptions(orgId ?? ''),
    enabled: Boolean(orgId),
  })
  const projectEnvQuery = useQuery({
    ...projectEnvQueryOptions(projectId ?? ''),
    enabled: Boolean(projectId),
  })
  const environmentEnvQuery = useQuery({
    ...environmentEnvQueryOptions(environmentId ?? ''),
    enabled: Boolean(environmentId),
  })

  const layers = useMemo(
    () => ({
      organization: orgEnvQuery.data,
      project: projectEnvQuery.data,
      environment: environmentEnvQuery.data,
    }),
    [orgEnvQuery.data, projectEnvQuery.data, environmentEnvQuery.data],
  )

  const inherited = useMemo(() => computeInheritedEnv(layers), [layers])
  const inheritedRows = useMemo(
    () =>
      inheritedOnlyEnv(layers, app.env).map((row) => ({
        key: row.key,
        value: row.value,
        badge: <Badge variant="muted">from {row.tier}</Badge>,
      })),
    [layers, app.env],
  )

  return (
    <div className="space-y-6">
      <EnvVarsForm
        title="Environment variables"
        description="Plain string values only. Secret references are not supported yet, use the Secrets card below for values that need to stay encrypted."
        emptyMessage="No environment variables set."
        pastePlaceholder="DATABASE_URL=postgres://localhost/app"
        values={app.env}
        isPending={updateApp.isPending}
        errorMessage={updateApp.isError ? updateApp.error.message : undefined}
        renderKeyBadge={(key) => {
          const shadowed = key ? inherited[key.trim()] : undefined
          return shadowed ? (
            <Badge variant="muted">
              own value &middot; overrides {shadowed.tier}
            </Badge>
          ) : (
            <Badge variant="outline">own value</Badge>
          )
        }}
        inheritedRows={inheritedRows}
        onSave={(env) => {
          updateApp.mutate(
            { ...app, env },
            {
              onSuccess: () => {
                notifyRestartRequired(app.name, 'Variables saved.')
              },
            },
          )
        }}
      />
      <EnvActivityPanel appName={app.name} />
    </div>
  )
}
