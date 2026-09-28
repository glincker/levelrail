import {
  ArrowLeftIcon,
  CheckCircleIcon,
  XCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { StatusPill } from './kit/StatusPill'
import type { NodeProvisionStatus } from '../types/nodeProvision'

// Shared step chrome for AddNodeWizard.tsx's own region/size/details/
// confirm steps: a back arrow in the title, scrollable body, one
// Continue action. Split into its own file purely to keep
// AddNodeWizard.tsx itself under this project's 500-line file guideline.
export function StepShell({
  title,
  onBack,
  onContinue,
  continueDisabled,
  continueLabel = 'Continue',
  children,
}: {
  title: string
  onBack: () => void
  onContinue: () => void
  continueDisabled?: boolean
  continueLabel?: string
  children: React.ReactNode
}) {
  return (
    <>
      <DialogHeader>
        <DialogTitle className="flex items-center gap-2">
          <button
            type="button"
            onClick={onBack}
            aria-label="Back"
            className="text-muted-foreground hover:text-foreground"
          >
            <ArrowLeftIcon className="size-4" />
          </button>
          {title}
        </DialogTitle>
      </DialogHeader>
      <div className="space-y-4">{children}</div>
      <DialogFooter>
        <Button type="button" onClick={onContinue} disabled={continueDisabled}>
          {continueLabel}
        </Button>
      </DialogFooter>
    </>
  )
}

const PROGRESS_STAGES: { status: NodeProvisionStatus; label: string }[] = [
  { status: 'creating', label: 'Creating server' },
  { status: 'booting', label: 'Booting' },
  { status: 'enrolling', label: 'Enrolling' },
  { status: 'ready', label: 'Ready' },
]

export function ProvisionProgress({
  status,
  failureReason,
}: {
  status: NodeProvisionStatus | undefined
  failureReason: string | undefined
}) {
  if (status === 'failed') {
    return (
      <Alert variant="destructive">
        <XCircleIcon />
        <AlertDescription>
          {failureReason ?? 'The provision failed.'}
        </AlertDescription>
      </Alert>
    )
  }
  const currentIndex = PROGRESS_STAGES.findIndex((s) => s.status === status)
  return (
    <ol className="space-y-2">
      {PROGRESS_STAGES.map((stage, i) => {
        const done = currentIndex > i || status === 'ready'
        const active = currentIndex === i && status !== 'ready'
        return (
          <li key={stage.status} className="flex items-center gap-2 text-sm">
            {done ? (
              <CheckCircleIcon className="size-4 text-tone-success" />
            ) : (
              <span
                className={`size-4 rounded-full border ${active ? 'border-primary' : 'border-border'}`}
              />
            )}
            <span
              className={
                done || active ? 'text-foreground' : 'text-muted-foreground'
              }
            >
              {stage.label}
            </span>
            {active ? (
              <StatusPill tone="info" label="in progress" live size="sm" />
            ) : null}
          </li>
        )
      })}
    </ol>
  )
}
