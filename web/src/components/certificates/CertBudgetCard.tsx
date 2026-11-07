import { useState, type FormEvent } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ShieldCheckGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Progress } from '@/components/ui/progress'
import { cn } from '@/lib/utils'
import {
  certBudgetQueryOptions,
  type CertBudget,
} from '../../queries/certBudget'
import { useSetRuntimeSetting } from '../../queries/runtimeSettings'

const WARN_KEY = 'certificates.weekly_issue_warn'
// Fraction of the threshold where the meter turns from fine to "close".
const NEAR_RATIO = 0.8

type Level = 'ok' | 'near' | 'over'

function levelOf(b: CertBudget): Level {
  if (b.over) return 'over'
  return b.issued >= b.warn * NEAR_RATIO ? 'near' : 'ok'
}

const BADGE = { ok: 'success', near: 'warning', over: 'destructive' } as const
const BAR = {
  ok: '[&_[data-slot=progress-indicator]]:bg-tone-success',
  near: '[&_[data-slot=progress-indicator]]:bg-tone-warning',
  over: '[&_[data-slot=progress-indicator]]:bg-tone-danger',
} as const

function ThresholdForm({ warn, canEdit }: { warn: number; canEdit: boolean }) {
  const { t } = useTranslation('certBudget')
  const save = useSetRuntimeSetting()
  const [draft, setDraft] = useState(String(warn))
  const [synced, setSynced] = useState(warn)
  // Follow the server value after a save or an outside change.
  if (synced !== warn) {
    setSynced(warn)
    setDraft(String(warn))
  }
  const parsed = Number(draft)
  const valid = Number.isInteger(parsed) && parsed >= 1
  const dirty = valid && parsed !== warn

  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (!dirty) return
    save.mutate({ key: WARN_KEY, value: String(parsed) })
  }

  let chip = null
  if (save.isPending) chip = t('budget.saving')
  else if (save.isError) chip = t('budget.saveFailed')
  else if (save.isSuccess && !dirty) chip = t('budget.saved')

  return (
    <form
      onSubmit={submit}
      className="flex flex-wrap items-center gap-x-3 gap-y-2"
    >
      <label htmlFor="cert-warn" className="text-sm font-medium">
        {t('budget.threshold')}
      </label>
      <Input
        id="cert-warn"
        type="number"
        min={1}
        inputMode="numeric"
        className="w-24"
        value={draft}
        disabled={!canEdit}
        aria-invalid={!valid}
        aria-describedby="cert-warn-help"
        onChange={(e) => {
          setDraft(e.target.value)
          save.reset()
        }}
      />
      <Button
        type="submit"
        size="sm"
        disabled={!canEdit || !dirty || save.isPending}
      >
        {t('budget.save')}
      </Button>
      <span
        role="status"
        aria-live="polite"
        className={cn(
          'text-xs',
          save.isError ? 'text-tone-danger' : 'text-muted-foreground',
        )}
      >
        {chip}
      </span>
      <span
        id="cert-warn-help"
        className={cn('text-xs text-muted-foreground', valid && 'sr-only')}
      >
        {valid ? t('budget.thresholdHelp') : t('budget.invalid')}
      </span>
    </form>
  )
}

export function CertBudgetCard({ canEdit = true }: { canEdit?: boolean }) {
  const { t } = useTranslation('certBudget')
  const q = useQuery(certBudgetQueryOptions())
  const b = q.data
  const level = b ? levelOf(b) : 'ok'

  return (
    <section
      aria-label={t('budget.title')}
      className="flex flex-wrap items-center gap-x-4 gap-y-2 rounded-lg border border-border bg-card px-4 py-3"
    >
      <h2 className="flex items-center gap-2 text-sm font-semibold">
        <ShieldCheckGlassIcon size="1.1em" className="text-tone-success" />
        {t('budget.title')}
      </h2>
      {q.isPending ? (
        <p role="status" className="text-sm text-muted-foreground">
          …
        </p>
      ) : q.isError || !b ? (
        <div className="flex flex-wrap items-center gap-3" role="alert">
          <span className="text-sm text-tone-danger">
            {t('budget.loadError')}
          </span>
          <Button size="sm" variant="outline" onClick={() => void q.refetch()}>
            {t('budget.retry')}
          </Button>
        </div>
      ) : (
        <>
          <Badge variant={BADGE[level]} title={t(`budget.body.${level}`)}>
            {t(`budget.status.${level}`)}
          </Badge>
          <Progress
            value={Math.min(b.issued, b.warn)}
            max={b.warn}
            aria-label={t('budget.meterLabel', { days: b.window_days })}
            className={cn('w-32 gap-1.5', BAR[level])}
          />
          <p className="text-sm">
            {t('budget.usage', {
              issued: b.issued,
              warn: b.warn,
              days: b.window_days,
            })}
          </p>
          {level === 'ok' ? null : (
            <p className="basis-full text-xs text-muted-foreground">
              {t(`budget.body.${level}`)}
            </p>
          )}
          <div className="ml-auto flex flex-wrap items-center gap-x-4 gap-y-2">
            <ThresholdForm warn={b.warn} canEdit={canEdit} />
            <Link
              to="/settings/system-status"
              className="text-sm text-brand underline-offset-4 hover:underline"
            >
              {t('budget.doctor')}
            </Link>
          </div>
        </>
      )}
    </section>
  )
}
