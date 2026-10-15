import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { PencilSimpleIcon, ScrollIcon } from '@phosphor-icons/react/dist/ssr'
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
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useIsRoot } from '../../hooks/useIsRoot'
import { useLogUsage } from '../../queries/logBudgets'
import type { LogUsageApp } from '../../types/logBudgets'
import { LogBudgetEditor } from './LogBudgetEditor'
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

function Row({ row, canEdit }: { row: LogUsageApp; canEdit: boolean }) {
  const { t } = useTranslation('logBudgets')
  const fmt = useFormatBytes()
  const [editing, setEditing] = useState(false)
  const pct = row.percent === null ? null : Math.min(row.percent, 100)
  return (
    <TableRow>
      <TableCell className="font-medium">{row.app}</TableCell>
      <TableCell className="tabular-nums">{fmt(row.bytes)}</TableCell>
      <TableCell className="tabular-nums">
        {t('rate', { size: fmt(row.rate_bytes_per_min) })}
      </TableCell>
      <TableCell>
        {editing ? (
          <LogBudgetEditor
            app={row.app}
            initialMb={
              row.budget_custom ? Math.round(row.budget_bytes / 2 ** 20) : 0
            }
            onDone={() => setEditing(false)}
          />
        ) : (
          <span className="flex items-center gap-2">
            <span className="tabular-nums">
              {row.budget_bytes > 0 ? fmt(row.budget_bytes) : t('unlimited')}
            </span>
            {row.budget_custom && (
              <Badge variant="muted">{t('customBudget')}</Badge>
            )}
            {canEdit && (
              <Button
                type="button"
                size="icon-sm"
                variant="ghost"
                aria-label={t('editFor', { app: row.app })}
                onClick={() => setEditing(true)}
              >
                <PencilSimpleIcon />
              </Button>
            )}
          </span>
        )}
      </TableCell>
      <TableCell className="w-40">
        {pct === null ? (
          <span className="text-muted-foreground">-</span>
        ) : (
          <Progress value={pct} className={barTone(pct)} />
        )}
      </TableCell>
      <TableCell className="tabular-nums">
        {row.forecast_days === null
          ? '-'
          : row.forecast_days === 0
            ? t('forecastReached')
            : t('forecast', { days: row.forecast_days.toFixed(1) })}
      </TableCell>
      <TableCell className="tabular-nums">
        {row.trimmed_lines > 0
          ? t('trimmed', { count: row.trimmed_lines })
          : '-'}
      </TableCell>
    </TableRow>
  )
}

/** LogVolumeCard lists stored log volume per app against its budget. */
export function LogVolumeCard() {
  const { t } = useTranslation('logBudgets')
  const fmt = useFormatBytes()
  const isRoot = useIsRoot()
  const usage = useLogUsage()
  const [shown, setShown] = useState(PAGE_SIZE)
  const apps = usage.data?.apps ?? []
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
            icon={<ScrollIcon className="size-4" />}
            title={t('emptyTitle')}
            description={t('emptyDescription')}
          />
        )}
        {apps.length > 0 && (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('columns.app')}</TableHead>
                <TableHead>{t('columns.stored')}</TableHead>
                <TableHead>{t('columns.rate')}</TableHead>
                <TableHead>{t('columns.budget')}</TableHead>
                <TableHead>{t('columns.usage')}</TableHead>
                <TableHead>{t('columns.forecast')}</TableHead>
                <TableHead>{t('columns.trimmed')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {apps.slice(0, shown).map((row) => (
                <Row key={row.app} row={row} canEdit={isRoot} />
              ))}
            </TableBody>
          </Table>
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
