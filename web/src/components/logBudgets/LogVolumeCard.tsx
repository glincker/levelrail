import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { PencilGlassIcon, ScrollGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { EmptyState } from '@/components/ui/empty-state'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { Progress } from '@/components/ui/progress'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { useIsRoot } from '../../hooks/useIsRoot'
import { useLogUsage } from '../../queries/logBudgets'
import type { LogUsageApp } from '../../types/logBudgets'
import { LogBudgetEditor } from './LogBudgetEditor'
import { TrimLogsButton } from './TrimLogsButton'
import { splitBytes } from './format'

const PAGE_SIZE = 25

function useFormatBytes() {
  const { t } = useTranslation('logBudgets')
  return (bytes: number) => {
    const { n, unit } = splitBytes(bytes)
    return t(`bytes.${unit}`, { n })
  }
}

function barTone(percent: number): string {
  if (percent >= 100) return '[&_[data-slot=progress-indicator]]:bg-destructive'
  if (percent >= 80)
    return '[&_[data-slot=progress-indicator]]:bg-tone-warning-solid'
  return ''
}

function BudgetCell({ row, canEdit }: { row: LogUsageApp; canEdit: boolean }) {
  const { t } = useTranslation('logBudgets')
  const fmt = useFormatBytes()
  const [editing, setEditing] = useState(false)
  if (editing) {
    return (
      <LogBudgetEditor
        app={row.app}
        initialMb={
          row.budget_custom ? Math.round(row.budget_bytes / 2 ** 20) : 0
        }
        onDone={() => setEditing(false)}
      />
    )
  }
  return (
    <span className="flex flex-wrap items-center gap-2">
      <span className="tabular-nums">
        {row.budget_bytes > 0 ? fmt(row.budget_bytes) : t('unlimited')}
      </span>
      {row.budget_custom && <Badge variant="muted">{t('customBudget')}</Badge>}
      {canEdit && (
        <Button
          type="button"
          size="icon-sm"
          variant="ghost"
          aria-label={t('editFor', { app: row.app })}
          onClick={() => setEditing(true)}
        >
          <PencilGlassIcon size="1.2em" />
        </Button>
      )}
      {canEdit && row.percent !== null && row.percent > 100 && (
        <TrimLogsButton app={row.app} />
      )}
    </span>
  )
}

function useColumns(canEdit: boolean): ResponsiveColumn<LogUsageApp>[] {
  const { t } = useTranslation('logBudgets')
  const fmt = useFormatBytes()
  return [
    {
      id: 'app',
      header: t('columns.app'),
      primary: true,
      cell: (row) => (
        <span className="block truncate font-medium" title={row.app}>
          {row.app}
        </span>
      ),
    },
    {
      id: 'stored',
      header: t('columns.stored'),
      width: 'w-24',
      cell: (row) => <span className="tabular-nums">{fmt(row.bytes)}</span>,
    },
    {
      id: 'rate',
      header: t('columns.rate'),
      priority: 3,
      width: 'w-28',
      cell: (row) => (
        <span className="tabular-nums">
          {t('rate', { size: fmt(row.rate_bytes_per_min) })}
        </span>
      ),
    },
    {
      id: 'budget',
      header: t('columns.budget'),
      width: 'w-52',
      cell: (row) => <BudgetCell row={row} canEdit={canEdit} />,
    },
    {
      id: 'usage',
      header: t('columns.usage'),
      priority: 2,
      width: 'w-36',
      cell: (row) => {
        const pct = row.percent === null ? null : Math.min(row.percent, 100)
        return pct === null ? (
          <span className="text-muted-foreground">-</span>
        ) : (
          <Progress value={pct} className={barTone(pct)} />
        )
      },
    },
    {
      id: 'forecast',
      header: t('columns.forecast'),
      priority: 3,
      width: 'w-28',
      cell: (row) => (
        <span className="tabular-nums">
          {row.forecast_days === null
            ? '-'
            : row.forecast_days === 0
              ? t('forecastReached')
              : t('forecast', { days: row.forecast_days.toFixed(1) })}
        </span>
      ),
    },
    {
      id: 'trimmed',
      header: t('columns.trimmed'),
      priority: 3,
      width: 'w-28',
      cell: (row) => (
        <span className="tabular-nums">
          {row.trimmed_lines > 0
            ? t('trimmed', { count: row.trimmed_lines })
            : '-'}
        </span>
      ),
    },
  ]
}

/** LogVolumeCard lists stored log volume per app against its budget. */
export function LogVolumeCard() {
  const { t } = useTranslation('logBudgets')
  const fmt = useFormatBytes()
  const isRoot = useIsRoot()
  const usage = useLogUsage()
  const [shown, setShown] = useState(PAGE_SIZE)
  const apps = usage.data?.apps ?? []
  const columns = useColumns(isRoot)
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('title')}</CardTitle>
        <CardDescription>{t('description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {usage.isError && (
          <Alert variant="destructive">
            <AlertDescription>{t('loadError')}</AlertDescription>
          </Alert>
        )}
        {usage.isPending && <TableSkeleton columnCount={7} rowCount={3} />}
        {usage.data && (
          <p className="text-sm text-muted-foreground">
            {usage.data.default_budget_bytes > 0
              ? t('defaultBudget', {
                  size: fmt(usage.data.default_budget_bytes),
                })
              : t('noDefaultBudget')}
          </p>
        )}
        {usage.data && apps.length === 0 && (
          <EmptyState
            compact
            icon={<ScrollGlassIcon className="size-4.5" />}
            title={t('emptyTitle')}
            description={t('emptyDescription')}
          />
        )}
        {apps.length > 0 && (
          <ResponsiveTable
            rows={apps.slice(0, shown)}
            columns={columns}
            getRowId={(row) => row.app}
            ariaLabel={t('title')}
          />
        )}
        {apps.length > shown && (
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => setShown((n) => n + PAGE_SIZE)}
          >
            {t('showMore')}
          </Button>
        )}
      </CardContent>
    </Card>
  )
}
