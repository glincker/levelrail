import type { AppDetail } from '../types/appDetail'
import { useUpdateApp } from '../queries/apps'
import { useRestartRequiredToast } from '../hooks/useRestartRequiredToast'
import { EnvVarsForm } from './EnvVarsForm'

// Deliberately plain string values only: internal/deploy.requireNoUnresolvedEnv
// still rejects app.yaml's `{ secret: true }` / `{ from: ... }` env
// references outright, and store.DesiredService.Env is a flat
// map[string]string with no room to represent a secret reference even
// if this editor tried.
export function EnvEditor({ app }: { app: AppDetail }) {
  const updateApp = useUpdateApp(app.name)
  const notifyRestartRequired = useRestartRequiredToast()

  return (
    <EnvVarsForm
      title="Environment variables"
      description="Plain string values only. Secret references are not supported yet, use the Secrets card below for values that need to stay encrypted."
      emptyMessage="No environment variables set."
      pastePlaceholder="DATABASE_URL=postgres://localhost/app"
      values={app.env}
      isPending={updateApp.isPending}
      errorMessage={updateApp.isError ? updateApp.error.message : undefined}
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
  )
}
