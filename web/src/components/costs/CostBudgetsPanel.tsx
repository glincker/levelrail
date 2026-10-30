import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { TrashGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Progress } from '@/components/ui/progress'
import { toast } from '@/components/ui/toast'
import { useStopApp } from '../../queries/apps'
import {
  useCostBudgets,
  useDeleteCostBudget,
  useUpdateCostBudget,
} from '../../queries/costExplorer'
import { formatMoney } from '../../lib/usageFormat'
import type {
  CostBudgetLevel,
  CostBudgetStatus,
} from '../../types/costExplorer'
import { AddCostBudget } from './AddCostBudget'

const LEVEL_VARIANT: Record<
  CostBudgetLevel,
  'success' | 'warning' | 'destructive' | 'info'
> = {
  '': 'success',
  warn: 'warning',
  over: 'destructive',
  forecast: 'info',
}

const MAX_PAUSE_BUTTONS = 3

function BudgetFixes({ b }: { b: CostBudgetStatus }) {
  const { t } = useTranslation('costs')
  const update = useUpdateCostBudget()
  const stop = useStopApp()
  const [pauseTarget, setPauseTarget] = useState<string | null>(null)
  const pausable = b.enforced === 'pause' ? [] : b.apps
  return (
    <div
      className="flex flex-wrap items-center gap-2"
      role="group"
      aria-label={t('fix.label')}
    >
      <Button
        size="sm"
        disabled={update.isPending}
        onClick={() =>
          update.mutate(
            { id: b.id, body: { monthly: b.suggested_monthly } },
            {
              onSuccess: () =>
                toast.add({
                  type: 'success',
                  title: t('fix.raised', {
                    amount: formatMoney(b.suggested_monthly, b.currency),
                  }),
                }),
            },
          )
        }
      >
        {t('fix.raise', {
          amount: formatMoney(b.suggested_monthly, b.currency),
        })}
      </Button>
      {pausable.slice(0, MAX_PAUSE_BUTTONS).map((app) => (
        <Button
          key={app}
          size="sm"
          variant="outline"
          onClick={() => setPauseTarget(app)}
        >
          {t('fix.pause', { app })}
        </Button>
      ))}
      <Button
        size="sm"
        variant="outline"
        render={<Link to="/resource-advisor" />}
      >
        {t('fix.rightSize')}
      </Button>
      <ConfirmDialog
        open={pauseTarget !== null}
        onOpenChange={(open) => !open && setPauseTarget(null)}
        title={t('fix.pauseTitle', { app: pauseTarget ?? '' })}
        description={t('fix.pauseDescription')}
        confirmLabel={t('fix.pauseConfirm')}
        pendingLabel={t('fix.pausing')}
        pending={stop.isPending}
        error={stop.isError ? stop.error.message : null}
        onConfirm={() => {
          if (pauseTarget === null) return
          stop.mutate(pauseTarget, { onSuccess: () => setPauseTarget(null) })
        }}
      />
      {update.isError ? (
        <p className="w-full text-sm text-destructive" role="alert">
          {t('budgets.error', { message: update.error.message })}
        </p>
      ) : null}
    </div>
  )
}

function BudgetRow({ b }: { b: CostBudgetStatus }) {
  const { t } = useTranslation('costs')
  const remove = useDeleteCostBudget()
  const [confirm, setConfirm] = useState(false)
  const level = b.level || 'ok'
  return (
    <li className="space-y-2 py-3 first:pt-0 last:pb-0">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="min-w-0">
          <p className="truncate text-sm font-medium">{b.name}</p>
          <p className="truncate text-xs text-muted-foreground">
            {t(`budgets.scopes.${b.scope}`)}: {b.scope_value}
          </p>
        </div>
        <div className="flex items-center gap-2">
          {b.enforced ? (
            <Badge variant="destructive">
              {b.enforced === 'pause'
                ? t('budgets.enforced.pause')
                : t('budgets.enforced.throttle')}
            </Badge>
          ) : null}
          <Badge variant={LEVEL_VARIANT[b.level]}>
            {t(`budgets.state.${level}`)}
          </Badge>
          <Button
            size="icon"
            variant="ghost"
            aria-label={t('budgets.remove', { name: b.name })}
            onClick={() => setConfirm(true)}
          >
            <TrashGlassIcon size="1.4em" />
          </Button>
        </div>
      </div>
      <Progress value={Math.min(100, b.pct)} />
      <p className="text-xs text-muted-foreground">
        {t('budgets.spent', {
          spent: formatMoney(b.spent, b.currency),
          budget: formatMoney(b.monthly, b.currency),
        })}
        {' · '}
        {t('budgets.forecast', {
          mid: formatMoney(b.forecast.mid, b.currency),
          low: formatMoney(b.forecast.low, b.currency),
          high: formatMoney(b.forecast.high, b.currency),
        })}
      </p>
      <p className="text-xs text-muted-foreground">
        {t('budgets.thresholdsLine', {
          levels: b.thresholds.join(', '),
          reached: b.highest > 0 ? b.highest : t('budgets.none'),
        })}
      </p>
      {b.level ? <BudgetFixes b={b} /> : null}
      <ConfirmDialog
        open={confirm}
        onOpenChange={setConfirm}
        tone="destructive"
        title={t('budgets.deleteTitle', { name: b.name })}
        description={t('budgets.deleteDescription')}
        confirmLabel={t('budgets.delete')}
        pending={remove.isPending}
        error={remove.isError ? remove.error.message : null}
        onConfirm={() =>
          remove.mutate(b.id, { onSuccess: () => setConfirm(false) })
        }
      />
    </li>
  )
}

// Named monthly budgets with alert thresholds, a 100% action and one click fixes.
export function CostBudgetsPanel() {
  const { t } = useTranslation('costs')
  const { data, isError } = useCostBudgets()
  if (isError) return null
  const budgets = data?.budgets ?? []
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('budgets.title')}</CardTitle>
        <p className="text-sm text-muted-foreground">
          {t('budgets.description')}
        </p>
      </CardHeader>
      <CardContent className="space-y-4">
        {budgets.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('budgets.empty')}</p>
        ) : (
          <ul className="divide-y divide-border">
            {budgets.map((b) => (
              <BudgetRow key={b.id} b={b} />
            ))}
          </ul>
        )}
        <AddCostBudget />
        <p className="text-xs text-muted-foreground">
          {t('budgets.alertHint')}
        </p>
      </CardContent>
    </Card>
  )
}
