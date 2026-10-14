import { useTranslation } from 'react-i18next'
import { Banner } from '@/components/ui/banner'
import { useMaintenanceBanner } from '../../queries/scheduledMaintenance'

const MAX_SHOWN = 2

// Dashboard notice for maintenance that is running or about to start. An
// upcoming window can be dismissed; an active one stays until it ends.
export function MaintenanceBanner() {
  const { t, i18n } = useTranslation('maintenance')
  const { data } = useMaintenanceBanner()
  if (!data || data.length === 0) return null
  const fmt = new Intl.DateTimeFormat(i18n.language, {
    dateStyle: 'medium',
    timeStyle: 'short',
  })

  return (
    <div className="flex flex-col gap-2 px-4 pt-2 md:px-6">
      {data.slice(0, MAX_SHOWN).map((w) => {
        const active = w.state === 'active'
        const scope =
          w.scope === 'all'
            ? t('scope.all')
            : t(`scope.${w.scope}`, { target: w.target ?? '' })
        return (
          <Banner
            key={w.id}
            variant={active ? 'warning' : 'info'}
            title={t(active ? 'banner.active' : 'banner.upcoming', {
              title: w.title,
              start: fmt.format(new Date(w.starts_at)),
              end: fmt.format(new Date(w.ends_at)),
            })}
            {...(active ? {} : { dismissId: `maintenance-${w.id}` })}
          >
            <p>{w.message}</p>
            <p className="text-muted-foreground">
              {scope}
              {w.freeze ? ` ${t('banner.freeze')}` : ''}
            </p>
          </Banner>
        )
      })}
      {data.length > MAX_SHOWN ? (
        <p className="text-xs text-muted-foreground">
          {t('banner.more', { count: data.length - MAX_SHOWN })}
        </p>
      ) : null}
    </div>
  )
}
