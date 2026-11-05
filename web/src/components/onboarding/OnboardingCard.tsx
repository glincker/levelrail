import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { buttonVariants, Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
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
    <Card>
      <CardContent className="flex flex-wrap items-center gap-x-4 gap-y-2">
        <div className="min-w-0 flex-1">
          <p className="text-sm font-semibold text-foreground">
            {resuming ? t('card.resumeTitle') : t('card.title')}
          </p>
          <p className="text-xs text-muted-foreground">
            {resuming
              ? t('card.resumeBody', {
                  current: stepIndex(data.current_step) + 1,
                  total: ONBOARDING_STEPS.length,
                })
              : t('card.body')}
          </p>
        </div>
        <Link to="/welcome" className={buttonVariants({ size: 'sm' })}>
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
      </CardContent>
    </Card>
  )
}
