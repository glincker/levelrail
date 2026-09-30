import {
  ArrowLeftIcon,
  CheckCircleIcon,
  XCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { StatusPill } from './kit/StatusPill'
import { WizardStepFooter } from './kit/WizardStepFooter'
import type { NodeProvisionStatus } from '../types/nodeProvision'
import type { SSHNodeProvisionStatus } from '../types/nodeSSHProvision'

// Shared step chrome for AddNodeWizard.tsx's own region/size/details/
// confirm steps: a back arrow in the title, scrollable body, one
// Continue action. Split into its own file purely to keep
// AddNodeWizard.tsx itself under this project's 500-line file guideline.
export function StepShell({
  title,
  onBack,
  onContinue,
  continueDisabled,
  continueReason,
  continueLabel = 'Continue',
  children,
}: {
  title: string
  onBack: () => void
  onContinue: () => void
  continueDisabled?: boolean
  /** Explains why Continue is disabled; ignored while continueDisabled is falsy. */
  continueReason?: string
  continueLabel?: string
  children: React.ReactNode
}) {
  return (
    <>
      <DialogHeader>
        <DialogTitle className="flex items-center gap-2">
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            onClick={onBack}
            aria-label="Back"
          >
            <ArrowLeftIcon />
          </Button>
          {title}
        </DialogTitle>
      </DialogHeader>
      <div className="space-y-4">{children}</div>
      <DialogFooter>
        <WizardStepFooter
          canContinue={!continueDisabled}
          reason={continueDisabled ? continueReason : undefined}
          onContinue={onContinue}
          continueLabel={continueLabel}
          reasonId="add-node-continue-reason"
        />
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

const SSH_PROGRESS_STAGES: { status: SSHNodeProvisionStatus; label: string }[] =
  [
    { status: 'connecting', label: 'Connecting' },
    { status: 'detecting', label: 'Detecting the host' },
    { status: 'installing', label: 'Installing the agent' },
    { status: 'enrolling', label: 'Enrolling' },
    { status: 'ready', label: 'Ready' },
  ]

// SSHProvisionProgress is ProvisionProgress's SSH-path counterpart: a
// different status vocabulary (SSHNodeProvisionStatus has no cloud
// "creating"/"booting" stages, but does reach "installing" in practice,
// unlike the cloud path, see that type's own doc comment), plus what a
// live SSH session gives that a cloud-init document never can: a
// detected OS/arch and a streamed install log, shown so this doesn't
// feel like a black box.
export function SSHProvisionProgress({
  status,
  detectedOS,
  detectedArch,
  log,
  failureReason,
}: {
  status: SSHNodeProvisionStatus | undefined
  detectedOS: string | undefined
  detectedArch: string | undefined
  log: string | undefined
  failureReason: string | undefined
}) {
  if (status === 'failed') {
    return (
      <div className="space-y-3">
        <Alert variant="destructive">
          <XCircleIcon />
          <AlertDescription>
            {failureReason ?? 'The provision failed.'}
          </AlertDescription>
        </Alert>
        {log ? <SSHProvisionLog log={log} /> : null}
      </div>
    )
  }
  const currentIndex = SSH_PROGRESS_STAGES.findIndex((s) => s.status === status)
  return (
    <div className="space-y-3">
      <ol className="space-y-2">
        {SSH_PROGRESS_STAGES.map((stage, i) => {
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
      {detectedOS ? (
        <p className="text-xs text-muted-foreground">
          Detected{' '}
          <span className="font-mono text-foreground">{detectedOS}</span>
          {detectedArch ? ` (${detectedArch})` : null}
        </p>
      ) : null}
      {log ? <SSHProvisionLog log={log} /> : null}
    </div>
  )
}

function SSHProvisionLog({ log }: { log: string }) {
  return (
    <pre className="max-h-40 overflow-y-auto rounded-lg border border-border bg-muted/30 p-2 font-mono text-[11px] whitespace-pre-wrap text-muted-foreground">
      {log}
    </pre>
  )
}
