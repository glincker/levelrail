import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { buttonVariants, Button } from '@/components/ui/button'
import {
  onboardingActive,
  useMeOnboarding,
  useSetOnboarding,
} from '../../queries/meOnboarding'
import { ONBOARDING_STEPS, stepIndex } from './onboardingSteps'

/** OnboardingCard invites a new user into /welcome from the dashboard; it renders nothing unless the server has the flow switched on. */
export function OnboardingCard() {
  const { t } = useTranslation('onboarding')
  const { data } = useMeOnboarding()
  const dismiss = useSetOnboarding()
  if (!onboardingActive(data)) return null
  if (data.state !== 'not_started' && data.state !== 'in_progress') return null
  const resuming = data.state === 'in_progress'
  return (
    <div
      data-testid="onboarding-note"
      className="flex flex-col items-start gap-3 rounded-xl bg-muted/50 px-4 py-3"
    >
      <p className="min-w-0 text-sm text-muted-foreground">
        <span className="block font-medium text-foreground">
          {resuming ? t('card.resumeTitle') : t('card.title')}
        </span>
        <span className="block">
          {resuming
            ? t('card.resumeBody', {
                current: stepIndex(data.current_step) + 1,
                total: ONBOARDING_STEPS.length,
              })
            : t('card.body')}
        </span>
      </p>
      <div className="flex flex-wrap items-center gap-2">
        <Link
          to="/welcome"
          className={buttonVariants({ size: 'sm', variant: 'outline' })}
        >
          {resuming ? t('card.resume') : t('card.start')}
        </Link>
        <Button
          type="button"
          size="sm"
          variant="ghost"
          disabled={dismiss.isPending}
          onClick={() => dismiss.mutate({ state: 'dismissed' })}
        >
          {t('card.dismiss')}
        </Button>
      </div>
    </div>
  )
}
