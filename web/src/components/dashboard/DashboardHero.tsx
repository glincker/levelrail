import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
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

  return (
    <PageTemplate
      width="wide"
      title={td('hero.title')}
      actions={<NewAppMenu />}
      banners={banners}
    >
      {children}
    </PageTemplate>
  )
}
