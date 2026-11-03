import { useState } from 'react'
import { Link, useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Button, buttonVariants } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import {
  OnboardingTransitionError,
  onboardingActive,
  useMeOnboarding,
  useResetOnboarding,
  useSetOnboarding,
  type MeOnboarding,
  type OnboardingUpdate,
} from '../../queries/meOnboarding'
import { FinishStep } from './FinishStep'
import { ONBOARDING_STEPS, stepIndex } from './onboardingSteps'
import { OnboardingShell } from './OnboardingShell'
import { PreferencesStep } from './PreferencesStep'
import { ProfileStep } from './ProfileStep'
import { WelcomeStep } from './WelcomeStep'

function useOnboardingUpdate() {
  const { t } = useTranslation('onboarding')
  const navigate = useNavigate()
  const set = useSetOnboarding()
  const send = (update: OnboardingUpdate, leave = false) => {
    set.mutate(update, {
      onSuccess: () => {
        if (leave) void navigate({ to: '/' })
      },
      onError: (e) => {
        toast.add({
          title:
            e instanceof OnboardingTransitionError
              ? t('errors.transition')
              : t('errors.save'),
          type: 'error',
        })
      },
    })
  }
  return { send, busy: set.isPending }
}

function ActiveFlow({ onboarding }: { onboarding: MeOnboarding }) {
  const { t } = useTranslation('onboarding')
  const { send, busy } = useOnboardingUpdate()
  const [index, setIndex] = useState(() => stepIndex(onboarding.current_step))
  const last = ONBOARDING_STEPS.length - 1
  const stepAt = (i: number) => ONBOARDING_STEPS[i] ?? ONBOARDING_STEPS[0]

  const go = (to: number) => {
    setIndex(to)
    send({ state: 'in_progress', current_step: stepAt(to) })
  }

  return (
    <OnboardingShell
      index={index}
      busy={busy}
      continueLabel={
        index === 0
          ? t('footer.start')
          : index === last
            ? t('footer.finish')
            : undefined
      }
      onBack={() => go(Math.max(0, index - 1))}
      onSkip={() =>
        send({ state: 'snoozed', current_step: stepAt(index) }, true)
      }
      onContinue={() => {
        if (index === last) send({ state: 'completed' }, true)
        else go(index + 1)
      }}
    >
      {index === 0 ? (
        <WelcomeStep
          busy={busy}
          onDismiss={() => send({ state: 'dismissed' }, true)}
        />
      ) : null}
      {index === 1 ? <ProfileStep /> : null}
      {index === 2 ? <PreferencesStep /> : null}
      {index === last ? <FinishStep /> : null}
    </OnboardingShell>
  )
}

function Closed() {
  const { t } = useTranslation('onboarding')
  const reset = useResetOnboarding()
  return (
    <div className="max-w-xl space-y-3">
      <p className="text-sm text-muted-foreground">{t('closed.body')}</p>
      <div className="flex gap-2">
        <Button
          type="button"
          variant="outline"
          disabled={reset.isPending}
          onClick={() => reset.mutate()}
        >
          {t('closed.restart')}
        </Button>
        <Link to="/" className={buttonVariants({ variant: 'ghost' })}>
          {t('closed.dashboard')}
        </Link>
      </div>
    </div>
  )
}

function Unavailable() {
  const { t } = useTranslation('onboarding')
  return (
    <div className="max-w-xl space-y-3">
      <p className="text-sm text-muted-foreground">{t('unavailable.body')}</p>
      <Link to="/" className={buttonVariants({ variant: 'outline' })}>
        {t('closed.dashboard')}
      </Link>
    </div>
  )
}

/** OnboardingFlow is the /welcome page body. A missing route or a disabled flow shows a plain pointer back to the dashboard. */
export function OnboardingFlow() {
  const { data, isPending } = useMeOnboarding()
  if (isPending) return <Skeleton className="h-64 w-full" />
  if (!onboardingActive(data)) return <Unavailable />
  if (data.state === 'completed' || data.state === 'dismissed') {
    return <Closed />
  }
  return <ActiveFlow onboarding={data} />
}
