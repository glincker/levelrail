import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { LayersGlassIcon } from '@thesvg/ui-react'
import { CardSkeleton } from '@/components/ui/card-skeleton'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from '@/components/ui/toast'
import {
  useAppRollout,
  useResetRolloutPolicy,
  useSaveRolloutPolicy,
  type RolloutPolicy,
} from '../queries/appRollout'
import { InfoTip } from './kit'

type Field = keyof RolloutPolicy
const FIELDS: Field[] = ['max_surge', 'max_unavailable', 'min_available']

const MAX_PERCENT = 100
const VALUE_PATTERN = /^\d+%?$/

type FieldError = 'format' | 'percent' | null

// Mirrors the server rules: a non-negative integer, or a percent from 0% to 100%.
function checkValue(raw: string): FieldError {
  const v = raw.trim()
  if (v === '') return null
  if (!VALUE_PATTERN.test(v)) return 'format'
  if (v.endsWith('%') && Number(v.slice(0, -1)) > MAX_PERCENT) return 'percent'
  return null
}

const isZeroValue = (raw: string) => /^0+%?$/.test(raw.trim())

function PolicyForm({
  appName,
  initial,
}: {
  appName: string
  initial: RolloutPolicy
}) {
  const { t } = useTranslation('deploys')
  const save = useSaveRolloutPolicy(appName)
  const reset = useResetRolloutPolicy(appName)
  const [draft, setDraft] = useState<RolloutPolicy>(initial)
  const [baseline, setBaseline] = useState<RolloutPolicy>(initial)
  const busy = save.isPending || reset.isPending
  const errors = FIELDS.map((f) => checkValue(draft[f] ?? ''))
  const bothZero =
    isZeroValue(draft.max_surge ?? '') &&
    isZeroValue(draft.max_unavailable ?? '')
  const valid = errors.every((e) => e === null) && !bothZero
  const dirty = FIELDS.some(
    (f) => (draft[f] ?? '').trim() !== (baseline[f] ?? '').trim(),
  )

  function fail(error: Error) {
    toast.add({
      title: t('rolloutPolicy.saveFailed'),
      description: error.message,
      type: 'error',
    })
  }

  function onSave() {
    const trimmed: RolloutPolicy = {}
    for (const f of FIELDS) {
      const v = (draft[f] ?? '').trim()
      if (v) trimmed[f] = v
    }
    save.mutate(trimmed, {
      onSuccess: () => {
        setBaseline(trimmed)
        toast.add({ title: t('rolloutPolicy.saved'), type: 'success' })
      },
      onError: fail,
    })
  }

  function onReset() {
    reset.mutate(undefined, {
      onSuccess: () => {
        setDraft({})
        setBaseline({})
        toast.add({ title: t('rolloutPolicy.resetDone'), type: 'success' })
      },
      onError: fail,
    })
  }

  return (
    <CardContent className="space-y-3">
      <p className="text-sm text-muted-foreground">{t('rolloutPolicy.hint')}</p>
      <div className="flex flex-wrap items-start gap-3">
        {FIELDS.map((f, i) => {
          const err = errors[i]
          return (
            <div key={f} className="space-y-1">
              <Label htmlFor={`rollout-${f}`}>{t(`rolloutPolicy.${f}`)}</Label>
              <Input
                id={`rollout-${f}`}
                className="w-28"
                value={draft[f] ?? ''}
                placeholder={t('rolloutPolicy.placeholder')}
                disabled={busy}
                aria-invalid={err !== null}
                aria-describedby={`rollout-${f}-msg`}
                onChange={(e) => setDraft({ ...draft, [f]: e.target.value })}
              />
              <p
                id={`rollout-${f}-msg`}
                role={err ? 'alert' : undefined}
                className={
                  err
                    ? 'text-xs text-destructive'
                    : 'text-xs text-muted-foreground'
                }
              >
                {err
                  ? t(`rolloutPolicy.error.${err}`)
                  : t('rolloutPolicy.format')}
              </p>
            </div>
          )
        })}
        <Button
          type="button"
          size="sm"
          className="mt-5"
          disabled={busy || !dirty || !valid}
          onClick={onSave}
        >
          {t('rolloutPolicy.save')}
        </Button>
        <Button
          type="button"
          size="sm"
          variant="ghost"
          className="mt-5"
          disabled={busy}
          onClick={onReset}
        >
          {t('rolloutPolicy.reset')}
        </Button>
      </div>
      {bothZero ? (
        <p role="alert" className="text-xs text-destructive">
          {t('rolloutPolicy.error.bothZero')}
        </p>
      ) : null}
    </CardContent>
  )
}

/** RolloutPolicyCard edits the rolling-update surge and availability budget, same block as app.yaml rollout. */
export function RolloutPolicyCard({ appName }: { appName: string }) {
  const { t } = useTranslation('deploys')
  const { data, isPending } = useAppRollout(appName)
  if (isPending) return <CardSkeleton titleWidth="w-56" rows={2} />
  // null means an older server without the route: nothing to edit.
  if (!data) return null
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <LayersGlassIcon className="size-5 text-muted-foreground" />
          {t('rolloutPolicy.title')}
          <InfoTip label={t('rolloutPolicy.infoLabel')}>
            {t('rolloutPolicy.infoBody')}
          </InfoTip>
        </CardTitle>
      </CardHeader>
      <PolicyForm key={appName} appName={appName} initial={data.policy ?? {}} />
    </Card>
  )
}
