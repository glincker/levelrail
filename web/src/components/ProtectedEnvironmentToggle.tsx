import { ShieldWarningIcon } from '@phosphor-icons/react/dist/ssr'
import { Switch } from '@/components/ui/switch'
import { useSetEnvironmentProtected } from '../queries/environments'
import { toast } from '@/components/ui/toast'

// Toggles an environment's protected flag (PATCH
// /api/v1/environments/{id}, internal/api/environments.go's
// handleUpdateEnvironment): requires confirm: true on a deploy,
// rollback, or promotion targeting an app tagged with this environment,
// the same acknowledge-before-proceeding friction
// ProtectedEnvironmentNotice surfaces on those actions.
export function ProtectedEnvironmentToggle({
  id,
  name,
  protectedFlag,
  projectId,
}: {
  id: string
  name: string
  protectedFlag: boolean
  projectId: string
}) {
  const setProtected = useSetEnvironmentProtected(projectId)

  return (
    <label className="flex items-center gap-2 text-sm text-muted-foreground">
      <ShieldWarningIcon
        className={protectedFlag ? 'size-4 text-destructive' : 'size-4'}
        aria-hidden="true"
      />
      Protected
      <Switch
        checked={protectedFlag}
        disabled={setProtected.isPending}
        aria-label={`${protectedFlag ? 'Unprotect' : 'Protect'} ${name}`}
        onCheckedChange={(next) => {
          setProtected.mutate(
            { id, protected: next },
            {
              onSuccess: (updated) => {
                toast.add({
                  title: `Environment "${updated.name}" is now ${updated.protected ? 'protected' : 'unprotected'}.`,
                  type: 'success',
                })
              },
              onError: (error) => {
                toast.add({
                  title: 'Could not update environment.',
                  description: error.message,
                  type: 'error',
                })
              },
            },
          )
        }}
      />
    </label>
  )
}
