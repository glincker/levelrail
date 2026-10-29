import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { GaugeGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import {
  appIngressLimitsQueryOptions,
  useAppIngressLimitsSave,
  type AppIngressLimits,
} from '../queries/ingressEdge'

const MIB = 1024 * 1024

function LimitsForm({
  appName,
  limits,
}: {
  appName: string
  limits: AppIngressLimits
}) {
  const { t } = useTranslation('domains')
  const save = useAppIngressLimitsSave(appName)
  const [raw, setRaw] = useState(
    limits.max_body_bytes === null ? '' : String(limits.max_body_bytes / MIB),
  )
  const n = Number(raw)
  const blank = raw.trim() === ''
  const valid = blank || (Number.isFinite(n) && n > 0)
  const instance = Math.round(limits.instance_max_body_bytes / MIB)

  function submit(value: number | null) {
    save.mutate(value, {
      onSuccess: () => {
        toast.add({ title: t('appLimits.saved'), type: 'success' })
      },
      onError: (e) => {
        toast.add({ title: e.message, type: 'error' })
      },
    })
  }

  return (
    <div className="space-y-3">
      <Field>
        <FieldLabel htmlFor="app-body-cap">{t('appLimits.body')}</FieldLabel>
        <div className="flex items-center gap-2">
          <Input
            id="app-body-cap"
            inputMode="decimal"
            className="w-28 font-mono"
            placeholder={String(instance)}
            aria-invalid={!valid}
            value={raw}
            onChange={(e) => {
              setRaw(e.target.value)
            }}
          />
          <span className="text-sm text-muted-foreground">
            {t('edge.unit.mib')}
          </span>
        </div>
        <FieldDescription>
          {valid ? t('appLimits.help', { instance }) : t('appLimits.invalid')}
        </FieldDescription>
      </Field>
      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          size="sm"
          disabled={!valid || save.isPending}
          onClick={() => {
            submit(blank ? null : Math.round(n * MIB))
          }}
        >
          {t('appLimits.save')}
        </Button>
        {limits.max_body_bytes !== null ? (
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={save.isPending}
            onClick={() => {
              setRaw('')
              submit(null)
            }}
          >
            {t('appLimits.reset')}
          </Button>
        ) : null}
      </div>
    </div>
  )
}

export function AppIngressLimitsCard({ appName }: { appName: string }) {
  const { t } = useTranslation('domains')
  const { data } = useQuery(appIngressLimitsQueryOptions(appName))

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <GaugeGlassIcon className="size-4.5" aria-hidden="true" />
          {t('appLimits.title')}
        </CardTitle>
        <CardDescription>{t('appLimits.description')}</CardDescription>
      </CardHeader>
      <CardContent>
        {data ? (
          <LimitsForm
            key={String(data.max_body_bytes)}
            appName={appName}
            limits={data}
          />
        ) : null}
      </CardContent>
    </Card>
  )
}
