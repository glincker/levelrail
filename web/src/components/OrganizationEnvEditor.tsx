import { useOrganizationEnv, useSetOrganizationEnv } from '../queries/organizationEnv'
import { EnvVarsForm } from './EnvVarsForm'
import { toast } from '@/components/ui/toast'

// The base layer every member project (and, once project_env_vars gets
// its own editor, every project's apps) inherits and can override, per
// internal/reconcile/application's resolveEnv.
export function OrganizationEnvEditor({
  organizationId,
}: {
  organizationId: string
}) {
  const { data: env } = useOrganizationEnv(organizationId)
  const setEnv = useSetOrganizationEnv(organizationId)

  return (
    <EnvVarsForm
      title="Shared env vars"
      description="Inherited by every project filed under this organization, and by extension every app in those projects. A project or app declaring the same key overrides the value set here."
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
