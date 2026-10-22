import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { PlusIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { dayPeriod } from '../../lib/dashboardHome'
import { useAuthUsername } from '../../hooks/useAuthUsername'
import { useBrand } from '../../hooks/useBrand'
import { useMinuteNow } from '../../hooks/useMinuteNow'
import { CreateResourceWizard } from '../CreateResourceWizard'
import { PageTemplate } from '../shell/PageTemplate'

export function DashboardHero({
  banners,
  children,
}: {
  banners?: ReactNode
  children: ReactNode
}) {
  const { t } = useTranslation()
  const { t: td } = useTranslation('dashboard')
  const brand = useBrand()
  const username = useAuthUsername()
  const now = useMinuteNow()
  const period = dayPeriod(new Date(now).getHours())
  const greeting = username
    ? td('hero.greetingNamed', { context: period, name: username })
    : td('hero.greeting', { context: period })

  return (
    <PageTemplate
      width="wide"
      titleSize="greeting"
      eyebrow={brand.Name}
      title={greeting}
      actions={
        <CreateResourceWizard
          trigger={
            <Button size="sm">
              <PlusIcon />
              {t('shell.newApp')}
            </Button>
          }
        />
      }
      banners={banners}
    >
      {children}
    </PageTemplate>
  )
}
