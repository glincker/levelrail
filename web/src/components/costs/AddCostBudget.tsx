import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from '@/components/ui/toast'
import { useAppListOptional } from '../../queries/apps'
import { useCreateCostBudget } from '../../queries/costExplorer'
import { useProjectListOptional } from '../../queries/projects'
import type {
  CostBudgetAction,
  CostBudgetScope,
} from '../../types/costExplorer'

const SCOPES: CostBudgetScope[] = ['project', 'tag', 'app']
const ACTIONS: CostBudgetAction[] = ['none', 'throttle', 'pause']
const DEFAULT_THRESHOLDS = '50, 80, 100'
const SELECT_CLASS =
  'h-9 w-full rounded-md border border-input bg-background px-2 text-sm'

function parseThresholds(raw: string): number[] | null {
  const parts = raw
    .split(',')
    .map((p) => p.trim())
    .filter(Boolean)
  const nums = parts.map(Number)
  if (nums.length === 0 || nums.some((n) => !Number.isInteger(n) || n < 1)) {
    return null
  }
  return nums
}

// Form for a named budget scoped to a project, tag or app.
export function AddCostBudget() {
  const { t } = useTranslation('costs')
  const create = useCreateCostBudget()
  const projects = useProjectListOptional().data ?? []
  const apps = useAppListOptional().data ?? []
  const [name, setName] = useState('')
  const [scope, setScope] = useState<CostBudgetScope>('project')
  const [value, setValue] = useState('')
  const [monthly, setMonthly] = useState('')
  const [thresholds, setThresholds] = useState(DEFAULT_THRESHOLDS)
  const [action, setAction] = useState<CostBudgetAction>('none')
  const ids = {
    name: useId(),
    scope: useId(),
    value: useId(),
    monthly: useId(),
    thresholds: useId(),
    action: useId(),
  }
  const amount = Number(monthly)
  const levels = parseThresholds(thresholds)
  const valid =
    name.trim() !== '' &&
    value !== '' &&
    Number.isFinite(amount) &&
    amount > 0 &&
    levels !== null

  function submit() {
    if (!levels) return
    create.mutate(
      {
        name: name.trim(),
        scope,
        scope_value: value,
        monthly: amount,
        thresholds: levels,
        action,
      },
      {
        onSuccess: () => {
          toast.add({ type: 'success', title: t('budgets.created') })
          setName('')
          setValue('')
          setMonthly('')
          setThresholds(DEFAULT_THRESHOLDS)
          setAction('none')
        },
      },
    )
  }

  return (
    <form
      className="space-y-3"
      onSubmit={(e) => {
        e.preventDefault()
        if (valid) submit()
      }}
    >
      <h3 className="text-sm font-medium">{t('budgets.addTitle')}</h3>
      <div className="grid gap-3 sm:grid-cols-3">
        <div className="space-y-1">
          <Label htmlFor={ids.name}>{t('budgets.name')}</Label>
          <Input
            id={ids.name}
            value={name}
            maxLength={80}
            onChange={(e) => setName(e.target.value)}
          />
        </div>
        <div className="space-y-1">
          <Label htmlFor={ids.scope}>{t('budgets.scope')}</Label>
          <select
            id={ids.scope}
            value={scope}
            onChange={(e) => {
              setScope(e.target.value as CostBudgetScope)
              setValue('')
            }}
            className={SELECT_CLASS}
          >
            {SCOPES.map((s) => (
              <option key={s} value={s}>
                {t(`budgets.scopes.${s}`)}
              </option>
            ))}
          </select>
        </div>
        <div className="space-y-1">
          <Label htmlFor={ids.value}>{t(`budgets.target.${scope}`)}</Label>
          {scope === 'tag' ? (
            <Input
              id={ids.value}
              value={value}
              onChange={(e) => setValue(e.target.value)}
            />
          ) : (
            <select
              id={ids.value}
              value={value}
              onChange={(e) => setValue(e.target.value)}
              className={SELECT_CLASS}
            >
              <option value="" />
              {scope === 'project'
                ? projects.map((p) => (
                    <option key={p.id} value={p.id}>
                      {p.name}
                    </option>
                  ))
                : apps.map((a) => (
                    <option key={a.name} value={a.name}>
                      {a.name}
                    </option>
                  ))}
            </select>
          )}
        </div>
      </div>
      <div className="grid gap-3 sm:grid-cols-3">
        <div className="space-y-1">
          <Label htmlFor={ids.monthly}>{t('budgets.monthly')}</Label>
          <Input
            id={ids.monthly}
            type="number"
            inputMode="decimal"
            min="0"
            value={monthly}
            onChange={(e) => setMonthly(e.target.value)}
          />
        </div>
        <div className="space-y-1">
          <Label htmlFor={ids.thresholds}>{t('budgets.thresholds')}</Label>
          <Input
            id={ids.thresholds}
            value={thresholds}
            aria-invalid={levels === null}
            onChange={(e) => setThresholds(e.target.value)}
          />
        </div>
        <div className="space-y-1">
          <Label htmlFor={ids.action}>{t('budgets.action')}</Label>
          <select
            id={ids.action}
            value={action}
            onChange={(e) => setAction(e.target.value as CostBudgetAction)}
            className={SELECT_CLASS}
          >
            {ACTIONS.map((a) => (
              <option key={a} value={a}>
                {t(`budgets.actions.${a}`)}
              </option>
            ))}
          </select>
        </div>
      </div>
      <p className="text-xs text-muted-foreground">{t('budgets.actionHint')}</p>
      {create.isError ? (
        <p className="text-sm text-destructive" role="alert">
          {t('budgets.error', { message: create.error.message })}
        </p>
      ) : null}
      <Button type="submit" size="sm" disabled={!valid || create.isPending}>
        {t('budgets.create')}
      </Button>
    </form>
  )
}
