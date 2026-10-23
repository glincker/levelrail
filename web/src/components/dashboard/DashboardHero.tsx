import type { ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { dayPeriod } from '../../lib/dashboardHome'
import { useAuthUsername } from '../../hooks/useAuthUsername'
import { useBrand } from '../../hooks/useBrand'
import { useMinuteNow } from '../../hooks/useMinuteNow'
import { sessionQueryOptions } from '../../queries/security'
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
  const session = useQuery(sessionQueryOptions()).data
  // Greet by first name when the account has a display name, else by login name.
  const firstName = session?.display_name?.trim().split(/\s+/)[0] || username
  const now = useMinuteNow()
  const period = dayPeriod(new Date(now).getHours())
  const greeting = firstName
    ? td('hero.greetingNamed', { context: period, name: firstName })
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
