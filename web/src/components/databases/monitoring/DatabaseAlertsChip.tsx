import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { useDatabaseAlerts } from '../../../queries/databaseAlerts'
import { DATABASE_ALERTS_ANCHOR } from './DatabaseAlertsCard'

// Renders nothing unless a health alert is firing for this database.
export function DatabaseAlertsChip({ databaseName }: { databaseName: string }) {
  const { t } = useTranslation('dbMonitoring')
  const { firing } = useDatabaseAlerts(databaseName)
  if (firing.length === 0) return null
  return (
    <Link
      to="/databases/$name/metrics"
      params={{ name: databaseName }}
      hash={DATABASE_ALERTS_ANCHOR}
      className="inline-flex"
    >
      <Badge variant="destructive">
        {t('alerts.firingChip', { count: firing.length })}
      </Badge>
    </Link>
  )
}
