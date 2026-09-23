import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { CheckIcon, SparkleIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { cn } from '@/lib/utils'
import { useBrand } from '../../hooks/useBrand'
import {
  onboardingQueryOptions,
  useCompleteOnboarding,
  useUpdateOnboardingProgress,
} from '../../queries/onboarding'
import {
  SETUP_STEPS,
  SETUP_STEP_META,
  nextStep,
  resumeStep,
  withStepStatus,
} from '../../lib/setupWizard'
import type {
  SetupStepId,
  SetupStepMap,
  SetupStepStatus,
} from '../../lib/setupWizard'
import { ServerCheckStep } from './ServerCheckStep'
import { DomainStep } from './DomainStep'
import { GitProviderStep } from './GitProviderStep'
import { FirstAppStep } from './FirstAppStep'
import { DoneStep } from './DoneStep'

/** SetupWizard is the first-run setup flow, resumable from its server-side progress. */
export function SetupWizard() {
  const brand = useBrand()
  const navigate = useNavigate()
  const { data: state } = useSuspenseQuery(onboardingQueryOptions())
  const [step, setStep] = useState<SetupStepId>(() =>
    resumeStep(state.current_step, state.steps),
  )
  const saveProgress = useUpdateOnboardingProgress()
  const complete = useCompleteOnboarding()
  const pending = saveProgress.isPending || complete.isPending

  function goTo(id: SetupStepId, steps: SetupStepMap = state.steps) {
    setStep(id)
    saveProgress.mutate({ current_step: id, steps })
  }

  function finishStep(status: SetupStepStatus) {
    goTo(nextStep(step), withStepStatus(state.steps, step, status))
  }

  function finishWizard() {
    const steps = withStepStatus(state.steps, 'done', 'completed')
    saveProgress.mutate(
      { current_step: 'done', steps },
      {
        onSuccess: () =>
          complete.mutate(undefined, {
            onSuccess: () => void navigate({ to: '/' }),
          }),
      },
    )
  }

  function dismiss() {
    complete.mutate()
  }

  const stepProps = {
    onContinue: () => finishStep('completed'),
    onSkip: () => finishStep('skipped'),
    pending,
  }

  return (
    <Card className="mx-auto max-w-3xl">
      <CardHeader className="flex-row items-start justify-between gap-3 space-y-0">
        <div className="space-y-1">
          <CardTitle className="flex items-center gap-2 text-lg">
            <SparkleIcon className="size-5 text-primary" />
            Set up {brand.Name}
          </CardTitle>
          <CardDescription>
            A few checks and choices to get from a fresh server to a live app.
            Progress is saved, so you can leave and come back.
          </CardDescription>
        </div>
        {state.completed ? null : (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={dismiss}
            disabled={pending}
          >
            Dismiss setup
          </Button>
        )}
      </CardHeader>
      <CardContent className="space-y-5">
        <nav aria-label="Setup steps">
          <ol className="flex flex-wrap gap-1.5">
            {SETUP_STEPS.map((id, i) => {
              const status = state.steps[id]
              const current = id === step
              return (
                <li key={id}>
                  <button
                    type="button"
                    onClick={() => goTo(id)}
                    aria-current={current ? 'step' : undefined}
                    className={cn(
                      'flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs transition-colors',
                      current
                        ? 'border-primary bg-primary/10 text-foreground'
                        : 'border-border text-muted-foreground hover:bg-muted/50',
                    )}
                  >
                    <span className="flex size-4 items-center justify-center rounded-full bg-muted text-[10px]">
                      {status === 'completed' ? (
                        <CheckIcon className="size-3" />
                      ) : (
                        i + 1
                      )}
                    </span>
                    {SETUP_STEP_META[id].title}
                    {status === 'skipped' ? (
                      <span className="sr-only">(skipped)</span>
                    ) : null}
                  </button>
                </li>
              )
            })}
          </ol>
        </nav>

        <h2 className="text-base font-semibold text-foreground">
          {SETUP_STEP_META[step].title}
          {SETUP_STEP_META[step].optional ? (
            <span className="ml-2 text-xs font-normal text-muted-foreground">
              Optional
            </span>
          ) : null}
        </h2>

        {step === 'server' ? <ServerCheckStep {...stepProps} /> : null}
        {step === 'domain' ? <DomainStep {...stepProps} /> : null}
        {step === 'git' ? <GitProviderStep {...stepProps} /> : null}
        {step === 'app' ? <FirstAppStep {...stepProps} /> : null}
        {step === 'done' ? (
          <DoneStep
            steps={state.steps}
            onGoToStep={(id) => goTo(id)}
            onFinish={finishWizard}
            pending={pending}
          />
        ) : null}

        {saveProgress.error ? (
          <p className="text-xs text-destructive">
            Could not save progress: {saveProgress.error.message}
          </p>
        ) : null}
      </CardContent>
    </Card>
  )
}
