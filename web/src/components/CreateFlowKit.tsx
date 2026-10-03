import type { ComponentProps, FormEvent, ReactNode } from 'react'
import { WarningIcon } from '@phosphor-icons/react/dist/ssr'
import {
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

// Shared chrome for the Create*Dialog family: header, mutation-error
// banner, step indicator, and submit footer, none of which was shared
// before this file. Distinct from the narrower CreateFormShell.tsx
// (the step-2 wizard Fields components' own <form> + draft notice).

export function CreateFlowHeader({
  icon,
  title,
  description,
}: {
  icon?: ReactNode
  title: ReactNode
  description?: ReactNode
}) {
  return (
    <DialogHeader>
      <DialogTitle className="flex items-center gap-2">
        {icon}
        {title}
      </DialogTitle>
      {description ? (
        <DialogDescription>{description}</DialogDescription>
      ) : null}
    </DialogHeader>
  )
}

export interface CreateFlowStep {
  id: string
  label: string
}

// "Step X of N: Label" plus a row of dots, for the handful of create
// flows that are genuinely sequential (fill a form, then see a
// one-time-only result) rather than one form whose fields just change
// with a picker. Most Create*Dialog components have no steps at all and
// never render this.
export function CreateFlowSteps({
  steps,
  currentIndex,
}: {
  steps: CreateFlowStep[]
  currentIndex: number
}) {
  const current = steps[currentIndex]
  return (
    <div
      className="flex items-center gap-2"
      aria-label={
        current
          ? `Step ${currentIndex + 1} of ${steps.length}: ${current.label}`
          : undefined
      }
    >
      <div className="flex items-center gap-1" aria-hidden="true">
        {steps.map((step, index) => (
          <span
            key={step.id}
            className={cn(
              'h-1.5 w-5 rounded-full transition-colors',
              index <= currentIndex ? 'bg-primary' : 'bg-muted',
            )}
          />
        ))}
      </div>
      <span className="text-xs text-muted-foreground">
        Step {currentIndex + 1} of {steps.length}
        {current ? `: ${current.label}` : null}
      </span>
    </div>
  )
}

// Standardized destructive banner for a create flow's mutation failure.
// Renders nothing when there is no message, so call sites can pass a
// mutation's error message straight through unconditionally.
export function CreateFlowError({ message }: { message?: string | null }) {
  if (!message) {
    return null
  }
  return (
    <Alert variant="destructive">
      <WarningIcon />
      <AlertDescription>{message}</AlertDescription>
    </Alert>
  )
}

// The submit button every create flow's DialogFooter ends with: disabled
// while the mutation is pending (or for any other caller-supplied
// reason), label swapped to a pending-tense form while it runs.
export function CreateFlowSubmitButton({
  pending,
  pendingLabel,
  disabled,
  children,
  ...props
}: {
  pending: boolean
  pendingLabel: ReactNode
  disabled?: boolean
} & Omit<ComponentProps<typeof Button>, 'disabled' | 'type'>) {
  return (
    <Button type="submit" disabled={pending || Boolean(disabled)} {...props}>
      {pending ? pendingLabel : children}
    </Button>
  )
}

// Composed shell for the common case: header, a <form> around the
// caller's fields, the standardized error banner, and a footer ending in
// CreateFlowSubmitButton. Covers most of the Create*Dialog family
// directly; a dialog with a branching empty state or a second
// (post-success) view instead composes the pieces above by hand, same as
// CreateTokenDialog does for its reveal-once step.
export function CreateFlowShell({
  icon,
  title,
  description,
  steps,
  currentStepIndex,
  onSubmit,
  error,
  submitLabel,
  submitPendingLabel,
  pending,
  submitDisabled,
  secondaryFooter,
  children,
}: {
  icon?: ReactNode
  title: ReactNode
  description?: ReactNode
  steps?: CreateFlowStep[]
  currentStepIndex?: number
  onSubmit: (e: FormEvent<HTMLFormElement>) => void
  error?: string | null
  submitLabel: ReactNode
  submitPendingLabel: ReactNode
  pending: boolean
  submitDisabled?: boolean
  secondaryFooter?: ReactNode
  children: ReactNode
}) {
  return (
    <>
      <CreateFlowHeader icon={icon} title={title} description={description} />
      {steps && currentStepIndex !== undefined ? (
        <CreateFlowSteps steps={steps} currentIndex={currentStepIndex} />
      ) : null}
      <form onSubmit={onSubmit} className="space-y-4">
        {children}
        <CreateFlowError message={error} />
        <DialogFooter>
          {secondaryFooter}
          <CreateFlowSubmitButton
            pending={pending}
            pendingLabel={submitPendingLabel}
            disabled={submitDisabled}
          >
            {submitLabel}
          </CreateFlowSubmitButton>
        </DialogFooter>
      </form>
    </>
  )
}
