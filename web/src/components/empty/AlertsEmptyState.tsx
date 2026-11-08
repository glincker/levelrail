import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { BellRingGlassIcon, PlusGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { GuidedEmptyState } from './GuidedEmptyState'

export const ALERTS_CLI_ARGS = 'apps alerts create web --kind domain_health'

/** AlertsEmptyState guides a truly empty alerts list to its first item. */
export function AlertsEmptyState({ total }: { total: number }) {
  const { t } = useTranslation('statePlaceholders')
  return (
    <GuidedEmptyState
      total={total}
      icon={<BellRingGlassIcon size="1.4em" />}
      title={t('guided.alerts.title')}
      description={t('guided.alerts.body')}
      cliArgs={ALERTS_CLI_ARGS}
      helpPath="/alerts"
      action={
        <Button size="sm" render={<Link to="/apps" />} nativeButton={false}>
          <PlusGlassIcon size="1.4em" aria-hidden="true" />
          {t('guided.alerts.action')}
        </Button>
      }
    />
  )
}
