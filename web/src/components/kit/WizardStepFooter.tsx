import { ArrowRightIcon, InfoIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'

export interface WizardStepFooterProps {
  canContinue: boolean
  /** Shown below Continue when it's disabled; ignored while canContinue is true. */
  reason?: string
  onContinue: () => void
  onSkip?: () => void
  continueLabel?: string
  pending?: boolean
  /** id for the reason paragraph, referenced by Continue's aria-describedby. */
  reasonId?: string
}

/** WizardStepFooter renders a wizard step's Continue action, an optional Skip, and why Continue is disabled. */
export function WizardStepFooter({
  canContinue,
  reason,
  onContinue,
  onSkip,
  continueLabel = 'Continue',
  pending,
  reasonId = 'wizard-continue-reason',
}: WizardStepFooterProps) {
  const showReason = !canContinue && Boolean(reason)
  return (
    <div className="flex w-full flex-col items-end gap-2">
      <div className="flex flex-wrap items-center justify-end gap-2">
        {onSkip ? (
          <Button
            type="button"
            variant="ghost"
            onClick={onSkip}
            disabled={pending}
          >
            Skip this step
          </Button>
        ) : null}
        <Button
          type="button"
          onClick={onContinue}
          disabled={!canContinue || pending}
          aria-describedby={showReason ? reasonId : undefined}
        >
          {continueLabel}
          <ArrowRightIcon />
        </Button>
      </div>
      {showReason ? (
        <p
          id={reasonId}
          aria-live="polite"
          className="flex items-start justify-end gap-1.5 text-right text-xs text-muted-foreground"
        >
          <InfoIcon className="mt-0.5 size-3.5 shrink-0" aria-hidden="true" />
          {reason}
        </p>
      ) : null}
    </div>
  )
}
