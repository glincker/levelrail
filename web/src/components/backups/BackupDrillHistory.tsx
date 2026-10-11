import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatDate } from '../../lib/format'
import { useBackupDrills } from '../../queries/backupProtection'
import type { BackupDrill } from '../../types/backupProtection'

function resourceName(d: BackupDrill): string {
  return d.resource_kind === 'volume'
    ? `${d.service_name ?? ''}/${d.volume_name ?? ''}`
    : (d.database_name ?? '')
}

function statusVariant(status: BackupDrill['status']) {
  if (status === 'passed') return 'success' as const
  if (status === 'failed') return 'destructive' as const
  return 'muted' as const
}

function Checks({ drill }: { drill: BackupDrill }) {
  const { t } = useTranslation('backups')
  const items: [string, boolean][] = [
    [t('drills.check.object'), drill.object_ok],
    [t('drills.check.checksum'), drill.checksum_ok],
    [t('drills.check.restore'), drill.restore_ok],
    [t('drills.check.content'), drill.content_ok],
  ]
  return (
    <ul className="flex flex-wrap gap-1">
      {items.map(([label, ok]) => (
        <li key={label}>
          <Badge variant={ok ? 'success' : 'muted'}>{label}</Badge>
        </li>
      ))}
    </ul>
  )
}

/** BackupDrillHistory lists restore drills, newest first. */
export function BackupDrillHistory({
  service,
  volume,
  database,
}: {
  service?: string
  volume?: string
  database?: string
}) {
  const { t } = useTranslation('backups')
  const drills = useBackupDrills({ service, volume, database, limit: 25 })

  return (
    <section className="space-y-3" aria-labelledby="drill-history-title">
      <div>
        <h2 id="drill-history-title" className="text-base font-medium">
          {t('drills.title')}
        </h2>
        <p className="text-sm text-muted-foreground">
          {t('drills.description')}
        </p>
      </div>
      {drills.isError ? (
        <p className="text-sm text-destructive">
          {t('drills.loadFailed', { error: drills.error.message })}
        </p>
      ) : (drills.data ?? []).length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('drills.empty')}</p>
      ) : (
        <div className="overflow-x-auto rounded-lg border border-border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('drills.columns.resource')}</TableHead>
                <TableHead>{t('drills.columns.trigger')}</TableHead>
                <TableHead>{t('drills.columns.result')}</TableHead>
                <TableHead>{t('drills.columns.checks')}</TableHead>
                <TableHead>{t('drills.columns.took')}</TableHead>
                <TableHead>{t('drills.columns.started')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {(drills.data ?? []).map((d) => (
                <TableRow key={d.id}>
                  <TableCell className="font-medium">
                    {resourceName(d)}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {t(`drills.trigger.${d.trigger}`)}
                  </TableCell>
                  <TableCell>
                    <div className="flex flex-col gap-1">
                      <Badge variant={statusVariant(d.status)}>
                        {t(`drills.status.${d.status}`)}
                      </Badge>
                      {d.status === 'failed' ? (
                        <span
                          className="max-w-[22rem] truncate text-xs text-destructive"
                          title={d.error}
                        >
                          {t('drills.stage', { stage: d.stage })}: {d.error}
                        </span>
                      ) : null}
                    </div>
                  </TableCell>
                  <TableCell>
                    <Checks drill={d} />
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {d.status === 'running'
                      ? '-'
                      : `${(d.duration_ms / 1000).toFixed(1)}s`}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {formatDate(d.started_at, '-')}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </section>
  )
}
