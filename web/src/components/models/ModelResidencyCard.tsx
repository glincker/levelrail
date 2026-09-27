import { useState } from 'react'
import { MoonIcon, SunIcon } from '@phosphor-icons/react/dist/ssr'
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
  useSleepModel,
  useWakeModel,
} from '../../queries/models'
import type { ModelResidency, ModelResource } from '../../types/models'

const POSITIVE_INT = /^[1-9][0-9]*$/

function stateLabel(model: ModelResource): string {
  if (model.residency === 'always') return 'Always loaded'
  switch (model.residency_state) {
    case 'asleep':
      return 'Idle, engine stopped'
    case 'waking':
      return 'Waking up'
    default:
      return 'Loaded'
  }
}

export function ModelResidencyCard({ model }: { model: ModelResource }) {
  const [mode, setMode] = useState<ModelResidency>(model.residency)
  const [minutes, setMinutes] = useState(
    model.idle_ttl_seconds > 0 ? String(model.idle_ttl_seconds / 60) : '',
  )
  const save = useSetModelResidency()
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
    <section aria-label="Residency" className="space-y-3">
      <div className="flex items-center gap-2">
        <h3 className="text-sm font-semibold">Residency</h3>
        <StatusPill
          tone={model.residency_state === 'asleep' ? 'neutral' : 'success'}
          label={stateLabel(model)}
          size="sm"
        />
        <InfoTip label="About residency">
          On demand stops the engine after the idle time, freeing its VRAM, and
          starts it on the first request, which waits while the engine loads (up
          to APP_MODEL_WAKE_WAIT, then a 503 with Retry-After). Ollama keeps
          models loaded forever, so the container is stopped to unload it.
        </InfoTip>
      </div>
      <div className="flex flex-wrap items-end gap-3">
        <Field>
          <FieldLabel htmlFor="residency-mode">Mode</FieldLabel>
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
              <SelectItem value="always">Always loaded</SelectItem>
              <SelectItem value="on_demand">On demand</SelectItem>
            </SelectContent>
          </Select>
        </Field>
        {mode === 'on_demand' ? (
          <Field>
            <FieldLabel htmlFor="residency-idle">Idle minutes</FieldLabel>
            <Input
              id="residency-idle"
              className="w-32"
              value={minutes}
              onChange={(e) => {
                setMinutes(e.target.value)
              }}
              placeholder={`${String(Math.round(model.effective_idle_ttl_seconds / 60))} (default)`}
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
                  toast.add({ title: 'Residency saved.', type: 'success' })
                },
                onError: fail('Could not save residency.'),
              },
            )
          }}
        >
          Save
        </Button>
        {onDemand ? (
          <>
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={wake.isPending || model.residency_state !== 'asleep'}
              onClick={() => {
                wake.mutate(model.name, { onError: fail('Could not wake.') })
              }}
            >
              <SunIcon aria-hidden="true" />
              Wake now
            </Button>
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={sleep.isPending || model.residency_state === 'asleep'}
              onClick={() => {
                sleep.mutate(model.name, { onError: fail('Could not sleep.') })
              }}
            >
              <MoonIcon aria-hidden="true" />
              Sleep now
            </Button>
          </>
        ) : null}
      </div>
      {onDemand ? (
        <p className="text-xs text-muted-foreground">
          {model.last_active_at ? (
            <>
              Last used <RelativeTime at={model.last_active_at} />. Stops after{' '}
              {Math.round(model.effective_idle_ttl_seconds / 60)} idle minutes.
            </>
          ) : (
            'Not used since it went idle.'
          )}
        </p>
      ) : null}
    </section>
  )
}
