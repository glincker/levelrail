import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { sortApps } from '../../lib/dashboardApps'
import { appListQueryOptions } from '../../queries/apps'
import { PhoneAppCard } from './PhoneAppCard'

/** Phone-only home: replaces the apps panel below the md breakpoint. */
export function PhoneStatusList() {
  const { t } = useTranslation('dashboard')
  const { data: apps = [] } = useQuery(appListQueryOptions())
  if (apps.length === 0) return null
  return (
    <ul
      aria-label={t('phoneHome.list')}
      className="flex flex-col gap-2 md:hidden"
    >
      {sortApps(apps).map((app) => (
        <PhoneAppCard key={app.name} app={app} />
      ))}
    </ul>
  )
}
