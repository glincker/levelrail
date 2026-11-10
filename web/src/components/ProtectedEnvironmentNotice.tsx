import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Alert } from '@/components/ui/alert'
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
    <Alert variant="warning" className="space-y-2">
      <div className="flex items-start gap-2">
        <TriangleAlertGlassIcon
          className="mt-0.5 size-6 shrink-0 text-tone-warning"
          aria-hidden="true"
        />
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
    </Alert>
  )
}
