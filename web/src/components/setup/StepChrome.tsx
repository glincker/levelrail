import type { ReactNode } from 'react'
import {
  ArrowRightIcon,
  CheckCircleIcon,
  CheckIcon,
  CircleIcon,
  CircleNotchIcon,
  CopyIcon,
  InfoIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { useCopyToClipboard } from '../../hooks/useCopyToClipboard'
import type { StepGate, SubStepState } from '../../lib/setupWizard'

/** StepFooter renders Continue plus, when it is disabled, the reason why. */
export function StepFooter({
  gate,
  onContinue,
  onSkip,
  continueLabel = 'Continue',
  pending,
}: {
  gate: StepGate
  onContinue: () => void
  onSkip?: () => void
  continueLabel?: string
  pending?: boolean
}) {
  return (
    <div className="space-y-2 border-t border-border pt-4">
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
          disabled={!gate.canContinue || pending}
          aria-describedby={
            gate.canContinue ? undefined : 'setup-continue-reason'
          }
        >
          {continueLabel}
          <ArrowRightIcon />
        </Button>
      </div>
      {gate.canContinue ? null : (
        <p
          id="setup-continue-reason"
          aria-live="polite"
          className="flex items-start justify-end gap-1.5 text-right text-xs text-muted-foreground"
        >
          <InfoIcon className="mt-0.5 size-3.5 shrink-0" aria-hidden="true" />
          {gate.reason}
        </p>
      )}
    </div>
  )
}

const SUB_STEP_ICON: Record<SubStepState, ReactNode> = {
  pending: <CircleIcon className="size-5 text-muted-foreground" />,
  waiting: (
    <CircleNotchIcon className="size-5 animate-spin text-primary motion-reduce:animate-none" />
  ),
  ok: <CheckCircleIcon className="size-5 text-green-600 dark:text-green-400" />,
  error: <WarningCircleIcon className="size-5 text-destructive" />,
}

/** SubStepRow is one live verification line with a state icon. */
export function SubStepRow({
  state,
  title,
  detail,
  children,
}: {
  state: SubStepState
  title: string
  detail: string
  children?: ReactNode
}) {
  return (
    <li className="flex items-start gap-3 py-2.5">
      <span className="mt-0.5 shrink-0" aria-hidden="true">
        {SUB_STEP_ICON[state]}
      </span>
      <div className="min-w-0 flex-1 space-y-1">
        <p className="text-sm font-medium text-foreground">
          {title}
          <span className="sr-only">: {state}</span>
        </p>
        <p className="text-xs text-muted-foreground">{detail}</p>
        {children}
      </div>
    </li>
  )
}

/** CopyValue shows a monospace value with a copy button. */
export function CopyValue({ value, label }: { value: string; label: string }) {
  const { copied, copy } = useCopyToClipboard()
  return (
    <div className="flex min-w-0 items-center gap-2 rounded-md bg-muted/50 px-2.5 py-1.5">
      <code className="min-w-0 flex-1 truncate text-xs">{value}</code>
      <Button
        type="button"
        size="sm"
        variant="ghost"
        onClick={() => copy(value)}
        aria-label={`Copy ${label}`}
      >
        {copied ? <CheckIcon /> : <CopyIcon />}
      </Button>
    </div>
  )
}
