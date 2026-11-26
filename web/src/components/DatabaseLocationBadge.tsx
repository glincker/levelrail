import { useTranslation } from 'react-i18next'
import { CloudGlassIcon, ServerGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { databaseLocation } from '../lib/databasesFilter'
import type { DatabaseListEntry } from '../types/databaseDetail'

// Says where a database runs using list fields only: this server, a remote
// node, or an external host Levelrail connects to but does not run.
export function DatabaseLocationBadge({
  database,
}: Readonly<{ database: DatabaseListEntry }>) {
  const { t } = useTranslation('databases')
  const where = databaseLocation(database)
  const label =
    where === 'hosted'
      ? t('location.hosted', { host: database.external?.host ?? '' })
      : where === 'node'
        ? t('location.node', {
            name: database.node_name || database.node_id || '',
          })
        : t('location.local')
  const Icon = where === 'hosted' ? CloudGlassIcon : ServerGlassIcon
  return (
    <Badge variant="outline" className="max-w-full gap-1 px-1.5 py-0">
      <Icon size="1.2em" aria-hidden="true" />
      <span className="truncate">{label}</span>
    </Badge>
  )
}
