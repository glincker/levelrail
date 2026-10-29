import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { MoonGlassIcon, SunGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { InfoTip, RelativeTime, StatusPill } from '@/components/kit'
import { Button } from '@/components/ui/button'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import {
  useSetModelResidency,
  useSetModelSwapGroup,
  useSleepModel,
  useWakeModel,
} from '../../queries/models'
import type { ModelResidency, ModelResource } from '../../types/models'

const POSITIVE_INT = /^[1-9][0-9]*$/

function stateLabel(model: ModelResource, t: TFunction<'models'>): string {
  if (model.residency === 'always') return t('residencyCard.alwaysLoaded')
  switch (model.residency_state) {
    case 'asleep':
      return t('residencyCard.stateAsleep')
    case 'waking':
      return t('residencyCard.stateWaking')
    default:
      return t('residencyCard.stateLoaded')
  }
}

export function ModelResidencyCard({ model }: { model: ModelResource }) {
  const { t } = useTranslation('models')
  const [mode, setMode] = useState<ModelResidency>(model.residency)
  const [minutes, setMinutes] = useState(
    model.idle_ttl_seconds > 0 ? String(model.idle_ttl_seconds / 60) : '',
  )
  const [swapGroup, setSwapGroup] = useState(model.swap_group ?? '')
  const save = useSetModelResidency()
  const saveSwapGroup = useSetModelSwapGroup()
  const wake = useWakeModel()
  const sleep = useSleepModel()
  const invalid =
    mode === 'on_demand' && minutes !== '' && !POSITIVE_INT.test(minutes)
  const onDemand = model.residency === 'on_demand'

  function fail(title: string) {
    return (error: Error) => {
      toast.add({ title, description: error.message, type: 'error' })
    }
  }

  return (
    <section aria-label={t('residencyCard.title')} className="space-y-3">
      <div className="flex items-center gap-2">
        <h3 className="text-sm font-semibold">{t('residencyCard.title')}</h3>
        <StatusPill
          tone={model.residency_state === 'asleep' ? 'neutral' : 'success'}
          label={stateLabel(model, t)}
          size="sm"
        />
        <InfoTip label={t('residencyCard.aboutResidency')}>
          {t('residencyCard.aboutResidencyBody')}
        </InfoTip>
      </div>
      <div className="flex flex-wrap items-end gap-3">
        <Field>
          <FieldLabel htmlFor="residency-mode">
            {t('residencyCard.mode')}
          </FieldLabel>
          <Select
            value={mode}
            onValueChange={(v) => {
              if (v === 'always' || v === 'on_demand') setMode(v)
            }}
          >
            <SelectTrigger id="residency-mode" className="w-44">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="always">
                {t('residencyCard.alwaysLoaded')}
              </SelectItem>
              <SelectItem value="on_demand">
                {t('residencyCard.onDemand')}
              </SelectItem>
            </SelectContent>
          </Select>
        </Field>
        {mode === 'on_demand' ? (
          <Field>
            <FieldLabel htmlFor="residency-idle">
              {t('residencyCard.idleMinutes')}
            </FieldLabel>
            <Input
              id="residency-idle"
              className="w-32"
              value={minutes}
              onChange={(e) => {
                setMinutes(e.target.value)
              }}
              placeholder={t('residencyCard.idlePlaceholder', {
                minutes: Math.round(model.effective_idle_ttl_seconds / 60),
              })}
              inputMode="numeric"
            />
          </Field>
        ) : null}
        <Button
          type="button"
          size="sm"
          disabled={invalid || save.isPending}
          onClick={() => {
            save.mutate(
              {
                name: model.name,
                residency: mode,
                idleTtlSeconds:
                  mode === 'on_demand' && minutes !== ''
                    ? Number(minutes) * 60
                    : 0,
              },
              {
                onSuccess: () => {
                  toast.add({
                    title: t('residencyCard.saved'),
                    type: 'success',
                  })
                },
                onError: fail(t('residencyCard.saveFailed')),
              },
            )
          }}
        >
          {t('residencyCard.save')}
        </Button>
        {onDemand ? (
          <>
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={wake.isPending || model.residency_state !== 'asleep'}
              onClick={() => {
                wake.mutate(model.name, {
                  onError: fail(t('residencyCard.wakeFailed')),
                })
              }}
            >
              <SunGlassIcon size="1.2em" aria-hidden="true" />
              {t('residencyCard.wakeNow')}
            </Button>
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={sleep.isPending || model.residency_state === 'asleep'}
              onClick={() => {
                sleep.mutate(model.name, {
                  onError: fail(t('residencyCard.sleepFailed')),
                })
              }}
            >
              <MoonGlassIcon size="1.2em" aria-hidden="true" />
              {t('residencyCard.sleepNow')}
            </Button>
          </>
        ) : null}
      </div>
      {onDemand ? (
        <p className="text-xs text-muted-foreground">
          {model.last_active_at ? (
            <>
              {t('residencyCard.lastUsedPrefix')}{' '}
              <RelativeTime at={model.last_active_at} />.{' '}
              {t('residencyCard.stopsAfter', {
                minutes: Math.round(model.effective_idle_ttl_seconds / 60),
              })}
            </>
          ) : (
            t('residencyCard.notUsed')
          )}
        </p>
      ) : null}
      <div className="space-y-2 border-t border-border pt-3">
        <div className="flex items-center gap-2">
          <h3 className="text-sm font-semibold">
            {t('residencyCard.swapGroup')}
          </h3>
          <InfoTip label={t('residencyCard.aboutSwap')}>
            {t('residencyCard.aboutSwapBody')}
          </InfoTip>
        </div>
        <div className="flex flex-wrap items-end gap-3">
          <Field>
            <FieldLabel htmlFor="swap-group-input">
              {t('residencyCard.groupName')}
            </FieldLabel>
            <Input
              id="swap-group-input"
              className="w-44"
              value={swapGroup}
              onChange={(e) => {
                setSwapGroup(e.target.value)
              }}
              placeholder={t('residencyCard.groupNone')}
            />
          </Field>
          <Button
            type="button"
            size="sm"
            disabled={saveSwapGroup.isPending}
            onClick={() => {
              saveSwapGroup.mutate(
                { name: model.name, swapGroup: swapGroup.trim() },
                {
                  onSuccess: () => {
                    toast.add({
                      title: t('residencyCard.swapSaved'),
                      type: 'success',
                    })
                  },
                  onError: fail(t('residencyCard.swapSaveFailed')),
                },
              )
            }}
          >
            {t('residencyCard.save')}
          </Button>
        </div>
        {model.shares_gpu_with && model.shares_gpu_with.length > 0 ? (
          <p className="flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
            {t('residencyCard.sharesGpu')}
            {model.shares_gpu_with.map((name) => (
              <Badge
                key={name}
                variant="muted"
                className="rounded-full text-[10px]"
              >
                {name}
              </Badge>
            ))}
          </p>
        ) : null}
      </div>
    </section>
  )
}
