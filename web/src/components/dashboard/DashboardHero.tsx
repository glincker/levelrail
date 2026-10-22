import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { dayPeriod } from '../../lib/dashboardHome'
import { useAuthUsername } from '../../hooks/useAuthUsername'
import { useBrand } from '../../hooks/useBrand'
import { useMinuteNow } from '../../hooks/useMinuteNow'
import { NewAppMenu } from '../NewAppMenu'
import { PageTemplate } from '../shell/PageTemplate'

export function DashboardHero({
  banners,
  children,
}: {
  banners?: ReactNode
  children: ReactNode
}) {
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
      actions={<NewAppMenu />}
      banners={banners}
    >
      {children}
    </PageTemplate>
  )
}
