import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  DatabaseGlassIcon,
  FolderGlassIcon,
  GlobeGlassIcon,
  ServerGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import {
  CenterWires,
  Group,
  WIRING_COLUMN,
  WIRING_GRID,
  WireItem,
} from '../../pages/wiringKit'
import { useDatabaseLinks } from '../../../queries/databaseLinks'
import type { DatabaseResource } from '../../../types/databaseDetail'

/** Left to right: the apps fed by this database, the database, and where it runs and who can reach it. */
export function DatabaseWiring({
  database,
  projectName,
}: Readonly<{ database: DatabaseResource; projectName?: string }>) {
  const { t } = useTranslation('databases')
  const links = useDatabaseLinks(database.name).data ?? []
  const publicOn = database.publicly_accessible === true
  const scheduled = Boolean(database.backup_schedule)
  return (
    <section
      aria-labelledby="db-wiring-title"
      className="@container space-y-2 rounded-lg border border-border bg-card p-3"
    >
      <h2 id="db-wiring-title" className="text-sm font-semibold">
        {t('wiring.title')}
      </h2>
      <div className={WIRING_GRID}>
        <div className={WIRING_COLUMN}>
          <Group
            title={t('wiring.apps')}
            count={links.length}
            tone={links.length > 0 ? 'ok' : 'off'}
            storageKey={`lr.wiring.db.${database.name}.apps`}
          >
            {links.length === 0 ? (
              <p className="text-xs text-muted-foreground">
                {t('wiring.noApps')}
              </p>
            ) : (
              <ul className="space-y-1">
                {links.map((l) => (
                  <li key={`${l.app}-${l.env_var}`} className="min-w-0 text-xs">
                    <Link
                      to="/apps/$name"
                      params={{ name: l.app }}
                      className="font-medium underline-offset-2 hover:underline"
                    >
                      {l.app}
                    </Link>
                    <span className="ml-1.5 font-mono text-muted-foreground">
                      {l.env_var}
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </Group>
        </div>
        <div className="relative min-w-0 space-y-2 rounded-lg border border-border bg-muted/30 p-3">
          <h3 className="truncate text-sm font-semibold">{database.name}</h3>
          <div className="flex flex-wrap gap-1">
            <Badge variant="muted">{database.engine}</Badge>
            <Badge variant="muted">{database.version}</Badge>
          </div>
          <CenterWires tone="ok" />
        </div>
        <div className={WIRING_COLUMN}>
          <h3 className="sr-only">{t('wiring.outTitle')}</h3>
          <WireItem
            tone={database.node_name ? 'ok' : 'off'}
            icon={<ServerGlassIcon />}
            label={t('wiring.node')}
            value={database.node_name ?? t('wiring.noNode')}
            chip={database.node_name ? t('wiring.on') : t('wiring.none')}
          />
          <WireItem
            tone={publicOn ? 'warn' : 'ok'}
            icon={<GlobeGlassIcon />}
            label={t('wiring.access')}
            value={
              publicOn
                ? t('wiring.publicOn', { port: database.public_port })
                : t('wiring.private')
            }
            chip={publicOn ? t('wiring.public') : t('wiring.private')}
          />
          <WireItem
            tone={scheduled ? 'ok' : 'off'}
            icon={<DatabaseGlassIcon />}
            label={t('wiring.backups')}
            value={database.backup_schedule ?? t('wiring.noBackups')}
            chip={scheduled ? t('wiring.on') : t('wiring.none')}
          />
          <WireItem
            tone={database.project_id ? 'ok' : 'off'}
            icon={<FolderGlassIcon />}
            label={t('wiring.project')}
            value={
              database.project_id
                ? (projectName ?? database.project_id)
                : t('wiring.noProject')
            }
            chip={database.project_id ? t('wiring.on') : t('wiring.none')}
          />
        </div>
      </div>
    </section>
  )
}
