import { WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { Checkbox } from '@/components/ui/checkbox'

// Gates a deploy-shaped action (deploy, rollback, promote) whose target
// app is tagged with a protected environment: the same acknowledge-and-
// enable pattern DatabasePublicAccessCard uses for Redis's no-auth
// public-access warning, applied here to internal/api's own confirm:
// true gate (deploys.go/promote.go's environmentNeedsConfirmation).
export function ProtectedEnvironmentNotice({
  id,
  environmentName,
  acknowledged,
  onAcknowledgedChange,
}: {
  id: string
  environmentName: string
  acknowledged: boolean
  onAcknowledgedChange: (next: boolean) => void
}) {
  return (
    <div className="space-y-2 rounded-lg border border-amber-200 bg-amber-50 p-2.5 text-amber-900 dark:border-amber-900/50 dark:bg-amber-900/20 dark:text-amber-200">
      <div className="flex items-start gap-2">
        <WarningIcon className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
        <p className="text-sm">
          &ldquo;{environmentName}&rdquo; is a protected environment. Confirm
          you mean to proceed.
        </p>
      </div>
      <label htmlFor={id} className="flex items-center gap-2 pl-6 text-sm">
        <Checkbox
          id={id}
          checked={acknowledged}
          onCheckedChange={(checked) => {
            onAcknowledgedChange(checked === true)
          }}
        />
        I understand and want to proceed.
      </label>
    </div>
  )
}
