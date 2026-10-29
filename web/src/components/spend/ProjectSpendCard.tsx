import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CirclePauseGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import { ApiError } from '../../lib/apiError'
import { formatMoney } from '../../lib/usageFormat'
import {
  useCostBudgets,
  useCreateCostBudget,
  useDeleteCostBudget,
  useUpdateCostBudget,
} from '../../queries/costExplorer'
import { budgetPauseUrl, usePausePreview } from '../../queries/spendCap'
import type { CostBudgetStatus, SpendGuardMode } from '../../types/costExplorer'
import { GuardModeField } from './GuardModeField'
import { PausePreviewDialog } from './PausePreviewDialog'
import { SpendMeter } from './SpendMeter'

function CapForm({
  budget,
  currency,
  projectId,
  projectName,
  onDone,
}: {
  budget: CostBudgetStatus | null
  currency: string
  projectId: string
  projectName: string
  onDone: () => void
}) {
  const { t } = useTranslation('budget')
  const create = useCreateCostBudget()
  const update = useUpdateCostBudget()
  const [amount, setAmount] = useState(budget ? String(budget.monthly) : '')
  const [guard, setGuard] = useState<SpendGuardMode>(
    budget?.guard_mode ?? 'alert',
  )
  const [error, setError] = useState<string | null>(null)
  const id = useId()
  const pending = create.isPending || update.isPending

  function submit(e: React.FormEvent) {
    e.preventDefault()
    const monthly = Number(amount)
    if (!Number.isFinite(monthly) || monthly <= 0) {
      setError(t('project.form.badAmount'))
      return
    }
    setError(null)
    const opts = {
      onSuccess: () => {
        toast.add({ title: t('project.saved'), type: 'success' })
        onDone()
      },
      onError: (err: Error) => setError(err.message),
    }
    if (budget) {
      update.mutate(
        { id: budget.id, body: { monthly, guard_mode: guard } },
        opts,
      )
    } else {
      create.mutate(
        {
          name: `spend cap: ${projectName}`,
          scope: 'project',
          scope_value: projectId,
          monthly,
          guard_mode: guard,
        },
        opts,
      )
    }
  }

  return (
    <form onSubmit={submit} className="space-y-3" noValidate>
      <div className="max-w-xs space-y-1.5">
        <Label htmlFor={id}>{t('project.form.amount', { currency })}</Label>
        <Input
          id={id}
          inputMode="decimal"
          value={amount}
          onChange={(e) => setAmount(e.target.value)}
        />
      </div>
      <GuardModeField name={`${id}-guard`} value={guard} onChange={setGuard} />
      {error ? (
        <Alert variant="destructive" role="alert">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}
      <div className="flex gap-2">
        <Button type="submit" size="sm" disabled={pending}>
          {pending ? t('project.form.saving') : t('project.form.save')}
        </Button>
        <Button type="button" size="sm" variant="ghost" onClick={onDone}>
          {t('project.form.cancel')}
        </Button>
      </div>
    </form>
  )
}

/** ProjectSpendCard shows a project's monthly cost cap: progress, projection, guard and a pause preview. */
export function ProjectSpendCard({
  projectId,
  projectName,
}: {
  projectId: string
  projectName: string
}) {
  const { t } = useTranslation('budget')
  const query = useCostBudgets()
  const remove = useDeleteCostBudget()
  const [editing, setEditing] = useState(false)
  const [previewOpen, setPreviewOpen] = useState(false)
  const budget =
    query.data?.budgets.find(
      (b) => b.scope === 'project' && b.scope_value === projectId,
    ) ?? null
  const preview = usePausePreview(budget ? budgetPauseUrl(budget.id) : '')
  const currency = query.data?.currency || 'USD'

  // Budgets are admin only: other callers get a 403 and see no card.
  if (query.error instanceof ApiError && query.error.status === 403) return null

  let body
  if (query.isPending) {
    body = <Skeleton className="h-12 w-full" aria-label={t('loading')} />
  } else if (query.isError) {
    body = (
      <Alert variant="destructive" role="alert">
        <AlertDescription className="flex flex-wrap items-center justify-between gap-2">
          <span>{t('project.loadError')}</span>
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => void query.refetch()}
          >
            {t('retry')}
          </Button>
        </AlertDescription>
      </Alert>
    )
  } else if (editing) {
    body = (
      <CapForm
        budget={budget}
        currency={currency}
        projectId={projectId}
        projectName={projectName}
        onDone={() => setEditing(false)}
      />
    )
  } else if (!budget) {
    body = (
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="min-w-0">
          <p className="text-sm font-medium">{t('project.empty.title')}</p>
          <p className="text-xs text-muted-foreground">
            {t('project.empty.body')}
          </p>
        </div>
        <Button type="button" size="sm" onClick={() => setEditing(true)}>
          {t('project.empty.action')}
        </Button>
      </div>
    )
  } else {
    const level = budget.level || 'ok'
    body = (
      <div className="space-y-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            <Badge
              variant={
                level === 'over'
                  ? 'destructive'
                  : level === 'ok'
                    ? 'success'
                    : 'warning'
              }
            >
              {t(`project.level.${level}`)}
            </Badge>
            <span className="text-xs text-muted-foreground">
              {t('project.resets', {
                date: new Date(budget.resets_at).toLocaleDateString(),
              })}
            </span>
            {budget.guard_mode === 'block_deploys' ? (
              <Badge variant="info">{t('guard.block_deploys.label')}</Badge>
            ) : null}
          </div>
          <div className="flex flex-wrap gap-2">
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => {
                setPreviewOpen(true)
                preview.mutate()
              }}
            >
              <CirclePauseGlassIcon size="1.2em" aria-hidden="true" />
              {t('actions.preview')}
            </Button>
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => setEditing(true)}
            >
              {t('project.edit')}
            </Button>
            <Button
              type="button"
              size="sm"
              variant="ghost"
              disabled={remove.isPending}
              onClick={() =>
                remove.mutate(budget.id, {
                  onSuccess: () =>
                    toast.add({ title: t('project.removed'), type: 'success' }),
                })
              }
            >
              {t('project.remove')}
            </Button>
          </div>
        </div>
        <SpendMeter
          label={t('usage.cost')}
          detail={t('project.summary', {
            spent: formatMoney(budget.spent, budget.currency),
            limit: formatMoney(budget.monthly, budget.currency),
          })}
          pct={budget.pct}
          projectedPct={budget.forecast_pct}
          projected={formatMoney(budget.forecast.mid, budget.currency)}
        />
      </div>
    )
  }

  return (
    <Card className="bg-linear-to-b from-card to-muted/50" size="sm">
      <CardHeader>
        <CardTitle>{t('project.title')}</CardTitle>
        <p className="text-sm text-muted-foreground">
          {t('project.description')}
        </p>
      </CardHeader>
      <CardContent>{body}</CardContent>
      <PausePreviewDialog
        open={previewOpen}
        onOpenChange={setPreviewOpen}
        preview={preview.data}
        pending={preview.isPending}
        error={preview.error?.message ?? null}
      />
    </Card>
  )
}
