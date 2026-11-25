import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { CopyField } from '@/components/traffic/CopyField'
import { engineHasBackups } from '@/lib/databaseCatalog'
import { ChangeDatabaseVersionDialog } from '../../ChangeDatabaseVersionDialog'
import { MoveToProjectDialog } from '../../MoveToProjectDialog'
import { MoveToThisServerButton, PlacementLabel } from '../../PlacementLabel'
import type { DatabaseResource } from '../../../types/databaseDetail'

interface Row {
  key: 'engine' | 'backups' | 'network' | 'access' | 'upgrades'
  status: string
  to: '/databases/$name/settings' | '/databases/$name/access'
  section?: 'general' | 'networking' | 'backups'
}

// Rows only restate fields the database resource already carries.
function useRows(db: DatabaseResource): Row[] {
  const { t } = useTranslation('databases')
  const backups = engineHasBackups(db.engine)
    ? db.backup_schedule
      ? t('home.overview.backupsOn', { schedule: db.backup_schedule })
      : t('home.overview.backupsOff')
    : t('home.overview.backupsNone')
  const network = db.publicly_accessible
    ? t('home.overview.networkPublic', { port: db.public_port ?? '' })
    : t('home.overview.networkPrivate')
  return [
    {
      key: 'engine',
      status: `${db.engine} ${db.version}`,
      to: '/databases/$name/settings',
      section: 'general',
    },
    {
      key: 'backups',
      status: backups,
      to: '/databases/$name/settings',
      section: 'backups',
    },
    {
      key: 'network',
      status: network,
      to: '/databases/$name/settings',
      section: 'networking',
    },
    {
      key: 'access',
      status: t('home.overview.accessHint'),
      to: '/databases/$name/access',
    },
    {
      key: 'upgrades',
      status: t('home.overview.upgradesHint'),
      to: '/databases/$name/settings',
      section: 'backups',
    },
  ]
}

export function ServiceSummary({ database }: { database: DatabaseResource }) {
  const { t } = useTranslation('databases')
  const rows = useRows(database)
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('home.overview.services')}</CardTitle>
      </CardHeader>
      <CardContent>
        <table className="w-full text-sm">
          <tbody>
            {rows.map((r) => (
              <tr
                key={r.key}
                className="border-t border-border first:border-t-0"
              >
                <th scope="row" className="py-2 pr-3 text-left font-medium">
                  {t(`home.overview.rows.${r.key}`)}
                </th>
                <td className="py-2 pr-3 text-muted-foreground">{r.status}</td>
                <td className="py-2 text-right">
                  <Link
                    to={r.to}
                    params={{ name: database.name }}
                    search={r.section ? { section: r.section } : undefined}
                    className="text-primary hover:underline"
                  >
                    {t('home.overview.manage')}
                  </Link>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </CardContent>
    </Card>
  )
}

function Fact({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <div className="space-y-1">
      <dt className="text-xs text-muted-foreground uppercase">{label}</dt>
      <dd className="flex items-center gap-2 text-sm text-foreground">
        {children}
      </dd>
    </div>
  )
}

export function KeyFacts({
  database,
  projectName,
}: {
  database: DatabaseResource
  projectName?: string
}) {
  const { t } = useTranslation('databases')
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('home.overview.facts')}</CardTitle>
      </CardHeader>
      <CardContent>
        <dl className="space-y-3">
          <Fact label={t('home.settings.general.name')}>
            <CopyField
              value={database.name}
              label={t('home.settings.general.copyName')}
            />
          </Fact>
          <Fact label={t('connection.engine')}>
            <span className="capitalize">{database.engine}</span>
          </Fact>
          <Fact label={t('overview.version')}>
            <span className="font-mono">{database.version}</span>
            <ChangeDatabaseVersionDialog
              name={database.name}
              engine={database.engine}
              currentVersion={database.version}
            />
          </Fact>
          <Fact label={t('overview.project')}>
            {database.project_id
              ? (projectName ?? database.project_id)
              : t('overview.noProject')}
            <MoveToProjectDialog
              kind="database"
              name={database.name}
              currentProjectId={database.project_id}
            />
          </Fact>
          <Fact label={t('overview.node')}>
            <PlacementLabel
              nodeId={database.node_id}
              nodeName={database.node_name}
              nodeIsLocal={database.node_is_local}
            />
            <MoveToThisServerButton
              kind="database"
              name={database.name}
              nodeId={database.node_id}
              nodeName={database.node_name}
              nodeIsLocal={database.node_is_local}
            />
          </Fact>
        </dl>
      </CardContent>
    </Card>
  )
}
