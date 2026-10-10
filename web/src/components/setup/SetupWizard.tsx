import { useMemo, useReducer } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useQuery, useSuspenseQuery } from '@tanstack/react-query'
import {
  onboardingQueryOptions,
  useCompleteOnboarding,
  useUpdateOnboardingProgress,
} from '../../queries/onboarding'
import { systemDoctorQueryOptions } from '../../queries/systemDoctor'
import { nextStep, resumeStep, withStepStatus } from '../../lib/setupWizard'
import { railProgress, setupNavReducer } from '../../lib/setupRail'
import { buildReadiness } from '../../lib/setupReadiness'
import type {
  SetupStepId,
  SetupStepMap,
  SetupStepStatus,
} from '../../lib/setupWizard'
import { ServerCheckStep } from './ServerCheckStep'
import { TopologyStep } from './TopologyStep'
import { DomainStep } from './DomainStep'
import { EmailStep } from './EmailStep'
import { GitProviderStep } from './GitProviderStep'
import { FirstAppStep } from './FirstAppStep'
import { DoneStep } from './DoneStep'
import { SetupRail } from './SetupRail'
import { SetupShell } from './SetupShell'

/** SetupWizard is the first-run setup flow, resumable from its server-side progress. */
export function SetupWizard() {
  const navigate = useNavigate()
  const { data: state } = useSuspenseQuery(onboardingQueryOptions())
  const [nav, dispatch] = useReducer(setupNavReducer, undefined, () => ({
    current: resumeStep(state.current_step, state.steps),
  }))
  const step = nav.current
  const saveProgress = useUpdateOnboardingProgress()
  const complete = useCompleteOnboarding()
  const pending = saveProgress.isPending || complete.isPending

  // Read-only view of the doctor cache so the rail can flag a server step that finished with warnings.
  const { data: doctor } = useQuery({
    ...systemDoctorQueryOptions(),
    enabled: false,
  })
  const attention = useMemo(() => {
    const ids = new Set<SetupStepId>()
    if (doctor && buildReadiness(doctor).warnings.length > 0) ids.add('server')
    return ids
  }, [doctor])

  function goTo(id: SetupStepId, steps: SetupStepMap = state.steps) {
    dispatch({ type: 'goto', id })
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

  const stepProps = {
    onContinue: () => finishStep('completed'),
    onSkip: () => finishStep('skipped'),
    pending,
  }

  return (
    <SetupShell
      current={step}
      settled={railProgress(state.steps).done}
      rail={
        <SetupRail
          current={step}
          steps={state.steps}
          attention={attention}
          onGoTo={(id) => goTo(id)}
        />
      }
      saving={saveProgress.isPending}
      saveError={saveProgress.error?.message}
      dismissable={!state.completed}
      dismissDisabled={pending}
      onDismiss={() => complete.mutate()}
    >
      {step === 'server' ? <ServerCheckStep {...stepProps} /> : null}
      {step === 'topology' ? <TopologyStep {...stepProps} /> : null}
      {step === 'domain' ? <DomainStep {...stepProps} /> : null}
      {step === 'email' ? <EmailStep {...stepProps} /> : null}
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
    </SetupShell>
  )
}
