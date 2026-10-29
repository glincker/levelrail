import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TrashGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Progress } from '@/components/ui/progress'
import { useOrganizationListOptional } from '../../queries/organizations'
import { useProjectListOptional } from '../../queries/projects'
import {
  useDeleteUsageBudget,
  useSaveUsageBudget,
  useUsageBudgets,
} from '../../queries/usage'
import { formatMoney } from '../../lib/usageFormat'
import type { BudgetLevel, UsageBudget } from '../../types/usage'

const LEVEL_VARIANT: Record<
  BudgetLevel,
  'success' | 'warning' | 'destructive' | 'info'
> = {
  '': 'success',
  warn: 'warning',
  over: 'destructive',
  forecast: 'info',
}

function BudgetRow({ b }: { b: UsageBudget }) {
  const { t } = useTranslation('usage')
  const remove = useDeleteUsageBudget()
  const level = b.level || 'ok'
  return (
    <li className="space-y-2 py-3 first:pt-0 last:pb-0">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="min-w-0 truncate text-sm font-medium">{b.name}</p>
        <div className="flex items-center gap-2">
          <Badge variant={LEVEL_VARIANT[b.level]}>
            {t(`budgets.state.${level}`)}
          </Badge>
          <Button
            size="icon"
            variant="ghost"
            aria-label={t('budgets.remove', { name: b.name })}
            disabled={remove.isPending}
            onClick={() => remove.mutate(b.id)}
          >
            <TrashGlassIcon size="1.2em" />
          </Button>
        </div>
      </div>
      <Progress value={Math.min(100, b.spent_pct)} />
      <p className="text-xs text-muted-foreground">
        {t('budgets.spent', {
          spent: formatMoney(b.spent, b.currency),
          budget: formatMoney(b.monthly, b.currency),
        })}
        {' · '}
        {t('budgets.forecast', { amount: formatMoney(b.forecast, b.currency) })}
      </p>
    </li>
  )
}

function AddBudget() {
  const { t } = useTranslation('usage')
  const save = useSaveUsageBudget()
  const projects = useProjectListOptional().data ?? []
  const teams = useOrganizationListOptional().data ?? []
  const [scope, setScope] = useState<'project' | 'team'>('project')
  const [target, setTarget] = useState('')
  const [monthly, setMonthly] = useState('')
  const [warn, setWarn] = useState('80')
  const targetId = useId()
  const monthlyId = useId()
  const warnId = useId()
  const options = scope === 'project' ? projects : teams
  const amount = Number(monthly)
  const valid = target !== '' && Number.isFinite(amount) && amount > 0

  function submit() {
    save.mutate(
      {
        scope,
        scope_id: target,
        monthly: amount,
        warn_pct: Math.min(100, Math.max(1, Number(warn) || 80)),
      },
      {
        onSuccess: () => {
          setTarget('')
          setMonthly('')
        },
      },
    )
  }

  return (
    <div className="space-y-3">
      <div
        className="flex flex-wrap gap-1"
        role="group"
        aria-label={t('budgets.scope')}
      >
        {(['project', 'team'] as const).map((s) => (
          <Button
            key={s}
            size="sm"
            variant={scope === s ? 'secondary' : 'ghost'}
            aria-pressed={scope === s}
            onClick={() => {
              setScope(s)
              setTarget('')
            }}
          >
            {t(s === 'project' ? 'budgets.scopeProject' : 'budgets.scopeTeam')}
          </Button>
        ))}
      </div>
      <div className="grid gap-3 sm:grid-cols-3">
        <div className="space-y-1">
          <Label htmlFor={targetId}>{t('budgets.target')}</Label>
          <select
            id={targetId}
            value={target}
            onChange={(e) => setTarget(e.target.value)}
            className="h-9 w-full rounded-md border border-input bg-background px-2 text-sm"
          >
            <option value="" />
            {options.map((o) => (
              <option key={o.id} value={o.id}>
                {o.name}
              </option>
            ))}
          </select>
        </div>
        <div className="space-y-1">
          <Label htmlFor={monthlyId}>{t('budgets.monthly')}</Label>
          <Input
            id={monthlyId}
            type="number"
            inputMode="decimal"
            min="0"
            value={monthly}
            onChange={(e) => setMonthly(e.target.value)}
          />
        </div>
        <div className="space-y-1">
          <Label htmlFor={warnId}>{t('budgets.warn')}</Label>
          <Input
            id={warnId}
            type="number"
            inputMode="numeric"
            min="1"
            max="100"
            value={warn}
            onChange={(e) => setWarn(e.target.value)}
          />
        </div>
      </div>
      {save.isError ? (
        <p className="text-sm text-destructive" role="alert">
          {t('budgets.error', { message: save.error.message })}
        </p>
      ) : null}
      <Button size="sm" onClick={submit} disabled={!valid || save.isPending}>
        {t('budgets.save')}
      </Button>
    </div>
  )
}

// Monthly project and team budgets with where each one stands.
export function BudgetsPanel() {
  const { t } = useTranslation('usage')
  const { data } = useUsageBudgets()
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
        <AddBudget />
        <p className="text-xs text-muted-foreground">
          {t('budgets.alertHint')}
        </p>
      </CardContent>
    </Card>
  )
}
