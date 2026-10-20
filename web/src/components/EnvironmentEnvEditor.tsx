import {
  useEnvironmentEnv,
  useSetEnvironmentEnv,
} from '../queries/environmentEnv'
import { EnvVarsForm } from './EnvVarsForm'
import { toast } from '@/components/ui/toast'

// The tier between the owning project's own shared env vars and every
// app tagged with this environment, per internal/reconcile/application's
// resolveEnv.
export function EnvironmentEnvEditor({
  environmentId,
}: Readonly<{
  environmentId: string
}>) {
  const { data: env } = useEnvironmentEnv(environmentId)
  const setEnv = useSetEnvironmentEnv(environmentId)

  return (
    <EnvVarsForm
      title="Shared env vars"
      description="Inherited by every app tagged with this environment, overriding the owning project's own shared env vars. An app declaring the same key overrides the value set here."
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
