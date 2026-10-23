import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Alert, AlertDescription } from '@/components/ui/alert'
import type {
  SpendCap,
  SpendCapAction,
  SpendCapUpdate,
  SpendGuardMode,
} from '../../queries/spendCap'
import { GuardModeField } from '../spend/GuardModeField'

const GIB = 1024 ** 3
const DEFAULT_THRESHOLDS = '50, 80, 100'
const DEFAULT_RPS = 5
const ACTIONS: SpendCapAction[] = ['none', 'throttle', 'pause']

interface Draft {
  egress: string
  cost: string
  thresholds: string
  action: SpendCapAction
  rps: string
  guard: SpendGuardMode
}

function toDraft(cap: SpendCap | null): Draft {
  return {
    egress: cap?.monthly_egress_bytes
      ? String(Math.round((cap.monthly_egress_bytes / GIB) * 100) / 100)
      : '',
    cost: cap?.monthly_cost_ceiling ? String(cap.monthly_cost_ceiling) : '',
    thresholds: cap?.thresholds.join(', ') ?? DEFAULT_THRESHOLDS,
    action: cap?.action ?? 'none',
    rps: String(cap?.throttle_rps || DEFAULT_RPS),
    guard: cap?.guard_mode ?? 'alert',
  }
}

function parseAmount(raw: string): number | null {
  if (raw.trim() === '') return 0
  const n = Number(raw)
  return Number.isFinite(n) && n >= 0 ? n : null
}

function parseThresholds(raw: string): number[] | null {
  const parts = raw
    .split(',')
    .map((p) => p.trim())
    .filter(Boolean)
  const nums = parts.map(Number)
  const ok = nums.every((n) => Number.isInteger(n) && n >= 1 && n <= 100)
  return ok && nums.length > 0 ? nums : null
}

type Build = { update: SpendCapUpdate } | { error: string }

function build(draft: Draft, t: (key: string) => string): Build {
  const egress = parseAmount(draft.egress)
  const cost = parseAmount(draft.cost)
  const thresholds = parseThresholds(draft.thresholds)
  const rps = Number(draft.rps)
  if (egress === null || cost === null) return { error: t('form.badNumber') }
  if (!thresholds) return { error: t('form.badThresholds') }
  if (egress === 0 && cost === 0) return { error: t('form.needLimit') }
  if (draft.action === 'throttle' && !(Number.isInteger(rps) && rps >= 1)) {
    return { error: t('form.badNumber') }
  }
  return {
    update: {
      monthly_egress_bytes: Math.round(egress * GIB),
      monthly_cost_ceiling: cost,
      thresholds,
      action: draft.action,
      throttle_rps: draft.action === 'throttle' ? rps : 0,
      guard_mode: draft.guard,
    },
  }
}

const ACTION_KEYS = {
  none: ['form.actionNone', 'form.actionNoneHelp'],
  throttle: ['form.actionThrottle', 'form.actionThrottleHelp'],
  pause: ['form.actionPause', 'form.actionPauseHelp'],
} as const

/** BudgetForm edits an app's cap; it validates locally and leaves the request to the caller. */
export function BudgetForm({
  cap,
  pending,
  serverError,
  onSave,
  onCancel,
}: {
  cap: SpendCap | null
  pending: boolean
  serverError: string | null
  onSave: (update: SpendCapUpdate) => void
  onCancel: () => void
}) {
  const { t } = useTranslation('budget')
  const [draft, setDraft] = useState(() => toDraft(cap))
  const [error, setError] = useState<string | null>(null)
  const set = (patch: Partial<Draft>) => setDraft((d) => ({ ...d, ...patch }))

  function submit(e: React.FormEvent) {
    e.preventDefault()
    const built = build(draft, (key) => t(key as 'form.badNumber'))
    if ('error' in built) {
      setError(built.error)
      return
    }
    setError(null)
    onSave(built.update)
  }

  const shown = error ?? serverError
  return (
    <form onSubmit={submit} className="space-y-4" noValidate>
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="space-y-1.5">
          <Label htmlFor="budget-egress">{t('form.egress')}</Label>
          <Input
            id="budget-egress"
            inputMode="decimal"
            value={draft.egress}
            onChange={(e) => set({ egress: e.target.value })}
            aria-describedby="budget-egress-help"
          />
          <p id="budget-egress-help" className="text-xs text-muted-foreground">
            {t('form.egressHelp')}
          </p>
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="budget-cost">{t('form.cost')}</Label>
          <Input
            id="budget-cost"
            inputMode="decimal"
            value={draft.cost}
            onChange={(e) => set({ cost: e.target.value })}
            aria-describedby="budget-cost-help"
          />
          <p id="budget-cost-help" className="text-xs text-muted-foreground">
            {t('form.costHelp')}
          </p>
        </div>
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="budget-thresholds">{t('form.thresholds')}</Label>
        <Input
          id="budget-thresholds"
          value={draft.thresholds}
          onChange={(e) => set({ thresholds: e.target.value })}
          aria-describedby="budget-thresholds-help"
        />
        <p
          id="budget-thresholds-help"
          className="text-xs text-muted-foreground"
        >
          {t('form.thresholdsHelp')}
        </p>
      </div>
      <fieldset className="space-y-2">
        <legend className="text-sm font-medium">{t('form.action')}</legend>
        <div className="grid gap-2 sm:grid-cols-3">
          {ACTIONS.map((a) => (
            <label
              key={a}
              className="flex cursor-pointer items-start gap-2 rounded-lg border border-border p-2.5 text-sm has-[:checked]:border-ring has-[:focus-visible]:ring-2 has-[:focus-visible]:ring-ring"
            >
              <input
                type="radio"
                name="budget-action"
                value={a}
                checked={draft.action === a}
                onChange={() => set({ action: a })}
                className="mt-0.5"
              />
              <span className="min-w-0 space-y-0.5">
                <span className="block font-medium">
                  {t(ACTION_KEYS[a][0])}
                </span>
                <span className="block text-xs text-muted-foreground">
                  {t(ACTION_KEYS[a][1])}
                </span>
              </span>
            </label>
          ))}
        </div>
      </fieldset>
      <GuardModeField
        name="budget-guard"
        value={draft.guard}
        onChange={(guard) => set({ guard })}
      />
      {draft.action === 'throttle' ? (
        <div className="max-w-xs space-y-1.5">
          <Label htmlFor="budget-rps">{t('form.rps')}</Label>
          <Input
            id="budget-rps"
            inputMode="numeric"
            value={draft.rps}
            onChange={(e) => set({ rps: e.target.value })}
          />
        </div>
      ) : null}
      {shown ? (
        <Alert variant="destructive" role="alert">
          <AlertDescription>{shown}</AlertDescription>
        </Alert>
      ) : null}
      <div className="flex gap-2">
        <Button type="submit" size="sm" disabled={pending}>
          {pending ? t('form.saving') : t('form.save')}
        </Button>
        <Button
          type="button"
          size="sm"
          variant="ghost"
          disabled={pending}
          onClick={onCancel}
        >
          {t('form.cancel')}
        </Button>
      </div>
    </form>
  )
}
