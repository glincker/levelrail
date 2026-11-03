import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { PageTemplate } from '../components/shell/PageTemplate'
import { OnboardingFlow } from '../components/onboarding/OnboardingFlow'

export const Route = createFileRoute('/welcome')({
  component: WelcomePage,
})

function WelcomePage() {
  const { t } = useTranslation('onboarding')
  return (
    <PageTemplate title={t('page.title')} description={t('page.description')}>
      <OnboardingFlow />
    </PageTemplate>
  )
}
