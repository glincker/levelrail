import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { StatusLabel } from './AppRow'
import { DatabaseLocationBadge } from './DatabaseLocationBadge'
import { EngineMark } from './EngineMark'

import type { DatabaseListEntry } from '../types/databaseDetail'

// Card view of the same rows as the table: logo, name, engine and version,
// status, location, backups, and the two quick links.
export function DatabaseCards({
  databases,
}: Readonly<{ databases: readonly DatabaseListEntry[] }>) {
  const { t } = useTranslation('databases')
  return (
    <ul className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
      {databases.map((db) => (
        <li
          key={db.name}
          className="flex flex-col gap-2 rounded-container border border-border bg-card p-3 shadow-card"
        >
          <div className="flex items-center gap-3">
            <span className="flex size-10 shrink-0 items-center justify-center rounded-lg border border-border bg-background">
              <EngineMark engine={db.engine} className="size-6" />
            </span>
            <span className="flex min-w-0 flex-col">
              <Link
                to="/databases/$name"
                params={{ name: db.name }}
                className="truncate text-sm font-medium text-foreground hover:underline"
              >
                {db.name}
              </Link>
              <span className="truncate font-mono text-xs text-muted-foreground">
                {db.engine}{' '}
                {db.external
                  ? `${db.external.host}:${db.external.port}`
                  : db.version}
              </span>
            </span>
          </div>
          <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <StatusLabel status={db.status} />
            <DatabaseLocationBadge database={db} />
            <span className="text-xs text-muted-foreground">
              {db.backup_schedule ? t('list.backupsOn') : t('list.backupsOff')}
            </span>
          </div>
          <div className="flex items-center gap-3 text-xs">
            <Link
              to="/databases/$name"
              params={{ name: db.name }}
              className="text-primary hover:underline"
            >
              {t('list.connect')}
            </Link>
            {db.provision_state ? null : (
              <Link
                to="/databases/$name/explorer"
                params={{ name: db.name }}
                className="text-primary hover:underline"
              >
                {t('list.explorer')}
              </Link>
            )}
          </div>
        </li>
      ))}
    </ul>
  )
}
