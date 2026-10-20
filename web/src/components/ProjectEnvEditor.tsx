import { useProjectEnv, useSetProjectEnv } from '../queries/projectEnv'
import { EnvVarsForm } from './EnvVarsForm'
import { toast } from '@/components/ui/toast'

// The tier between the owning organization's own shared env vars and
// every app filed under this project, per internal/reconcile/
// application's resolveEnv: applied after the organization layer, so a
// project default overrides a same-named organization default, and
// before an environment's own shared env vars and the app's own Env,
// both of which override this project's.
export function ProjectEnvEditor({ projectId }: { projectId: string }) {
  const { data: env } = useProjectEnv(projectId)
  const setEnv = useSetProjectEnv(projectId)

  return (
    <EnvVarsForm
      title="Shared env vars"
      description="Inherited by every app filed under this project, overriding the owning organization's own shared env vars. An environment or app declaring the same key overrides the value set here."
      emptyMessage="No shared env vars set."
      pastePlaceholder="LOG_LEVEL=info"
      values={env}
      isPending={setEnv.isPending}
      errorMessage={setEnv.isError ? setEnv.error.message : undefined}
      onSave={(vars) => {
        setEnv.mutate(vars, {
          onSuccess: () => {
            toast.add({ title: 'Variables saved.', type: 'success' })
          },
        })
      }}
    />
  )
}
