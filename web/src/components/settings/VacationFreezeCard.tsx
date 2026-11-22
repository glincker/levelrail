import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Banner } from '@/components/ui/banner'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import {
  useSetVacationFreeze,
  useVacationFreeze,
} from '../../queries/vacationFreeze'

const DEFAULT_DAYS = '7'

export function VacationFreezeCard({ isRoot }: { isRoot: boolean }) {
  const { t } = useTranslation('vacationFreeze')
  const { data } = useVacationFreeze()
  const set = useSetVacationFreeze()
  const [days, setDays] = useState(DEFAULT_DAYS)
  const [reason, setReason] = useState('')
  const [error, setError] = useState<string | null>(null)
  const active = data?.active ?? false

  function toggle(on: boolean) {
    const n = Number(days)
    if (on && (!Number.isInteger(n) || n < 1)) {
      setError(t('daysInvalid'))
      return
    }
    setError(null)
    set.mutate(on ? { days: n, reason: reason.trim() || undefined } : null, {
      onSuccess: () =>
        toast.add({ type: 'success', title: on ? t('on') : t('off') }),
      onError: (err) => toast.add({ type: 'error', title: err.message }),
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('title')}</CardTitle>
        <p className="text-sm text-muted-foreground">{t('description')}</p>
      </CardHeader>
      <CardContent className="space-y-3">
        <Field orientation="horizontal">
          <Switch
            id="vacation-freeze-switch"
            checked={active}
            disabled={!isRoot || set.isPending}
            onCheckedChange={toggle}
          />
          <FieldLabel htmlFor="vacation-freeze-switch">
            {t('switchLabel')}
          </FieldLabel>
        </Field>
        {active ? null : (
          <div className="grid gap-3 sm:grid-cols-[8rem_1fr]">
            <Field data-invalid={error ? true : undefined}>
              <FieldLabel htmlFor="vacation-days">{t('days')}</FieldLabel>
              <Input
                id="vacation-days"
                type="number"
                min={1}
                value={days}
                disabled={!isRoot}
                aria-invalid={!!error}
                onChange={(e) => {
                  setDays(e.target.value)
                }}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="vacation-reason">{t('reason')}</FieldLabel>
              <Input
                id="vacation-reason"
                value={reason}
                placeholder={t('reasonPlaceholder')}
                disabled={!isRoot}
                onChange={(e) => {
                  setReason(e.target.value)
                }}
              />
            </Field>
          </div>
        )}
        {error ? <Banner variant="critical" title={error} /> : null}
        {isRoot ? null : <Banner variant="info" title={t('rootOnly')} />}
      </CardContent>
    </Card>
  )
}
