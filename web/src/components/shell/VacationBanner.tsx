import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { InfoGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { useVacationFreeze } from '../../queries/vacationFreeze'

// Shown on every page while the vacation freeze holds deploys and jobs.
export function VacationBanner() {
  const { t } = useTranslation('vacationFreeze', { useSuspense: false })
  const { data } = useVacationFreeze()
  if (!data?.active || !data.until) return null
  return (
    <div
      role="status"
      className="flex min-h-9 shrink-0 items-center gap-2 border-b border-warning/30 bg-warning/10 px-4 py-1 text-sm"
    >
      <span aria-hidden="true" className="[&_svg]:size-5">
        <InfoGlassIcon className="text-tone-warning" size="1.4em" />
      </span>
      <span className="min-w-0 flex-1 break-words">
        {t('active', {
          count: data.days_left,
          until: new Date(data.until).toLocaleDateString(),
        })}
      </span>
      <Button
        size="sm"
        variant="outline"
        render={<Link to="/settings/deploy-policies" />}
      >
        {t('details')}
      </Button>
    </div>
  )
}
