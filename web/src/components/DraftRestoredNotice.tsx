import { XIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertAction, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'

// Shared banner for every create-resource step-2 form's draft-restore
// flow (see hooks/useFormDraft.ts): a saved draft is restored silently on
// mount, this just surfaces that it happened and offers a way back to a
// blank form.
export function DraftRestoredNotice({
  onDiscard,
  onDismiss,
}: {
  onDiscard: () => void
  onDismiss: () => void
}) {
  return (
    <Alert>
      <AlertDescription>
        Restored your unsaved draft from earlier.
      </AlertDescription>
      <AlertAction className="flex items-center gap-1">
        <Button type="button" variant="outline" size="sm" onClick={onDiscard}>
          Discard draft
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          onClick={onDismiss}
          aria-label="Dismiss"
        >
          <XIcon />
        </Button>
      </AlertAction>
    </Alert>
  )
}
