import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { ArrowCircleUpIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { EmptyState } from '@/components/ui/empty-state'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import type { DatabaseUpgrades } from '../../types/databaseUpgrades'
import { UpgradeNowDialog } from './UpgradeNowDialog'
import { kindVariant } from './upgradeHelpers'

/** TargetsCard lists the versions this database can move to, with a typed-confirm upgrade for patch and minor. */
export function TargetsCard({
  databaseName,
  data,
}: {
  databaseName: string
  data: DatabaseUpgrades
}) {
  const { t } = useTranslation('databaseUpgrades')
  const { targets } = data.advice
  const busy = Boolean(data.active) || data.blockers.length > 0
  const columns = useMemo<ResponsiveColumn<(typeof targets)[number]>[]>(
    () => [
      {
        id: 'version',
        header: t('targets.columns.version'),
        primary: true,
        cell: (target) => <span className="font-mono">{target.version}</span>,
      },
      {
        id: 'kind',
        header: t('targets.columns.kind'),
        cell: (target) => (
          <Badge variant={kindVariant(target.kind)}>
            {t(`targets.kind.${target.kind}`)}
          </Badge>
        ),
      },
      {
        id: 'details',
        header: t('targets.columns.details'),
        priority: 2,
        cell: (target) => (
          <div className="flex flex-wrap items-center gap-1.5">
            {target.security ? (
              <Badge variant="destructive">{t('targets.security')}</Badge>
            ) : null}
            {(target.advisories ?? []).map((id) => (
              <Badge key={id} variant="outline" className="font-mono">
                {id}
              </Badge>
            ))}
            {target.automatic ? (
              <Badge variant="muted">{t('targets.automatic')}</Badge>
            ) : null}
            {target.eol ? (
              <span className="text-xs text-muted-foreground">
                {t('targets.eol', { date: target.eol })}
              </span>
            ) : null}
          </div>
        ),
      },
    ],
    [t],
  )

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('targets.title')}</CardTitle>
        <CardDescription>{t('targets.description')}</CardDescription>
      </CardHeader>
      <CardContent>
        {targets.length === 0 ? (
          <EmptyState
            className="gap-2 border-0 bg-transparent py-4"
            icon={<ArrowCircleUpIcon className="size-5" />}
            title={t('targets.emptyTitle')}
            description={t('targets.empty')}
          />
        ) : (
          <ResponsiveTable
            rows={targets}
            columns={columns}
            getRowId={(target) => target.version}
            ariaLabel={t('targets.tableLabel')}
            renderActions={(target) =>
              target.kind === 'major' ? (
                <span className="text-xs text-muted-foreground">
                  {t('targets.majorHint')}
                </span>
              ) : (
                <UpgradeNowDialog
                  databaseName={databaseName}
                  version={target.version}
                  kind={target.kind}
                  disabled={busy}
                />
              )
            }
          />
        )}
      </CardContent>
    </Card>
  )
}
