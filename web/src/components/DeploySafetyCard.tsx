import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ShieldCheckGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import { InfoTip } from '@/components/kit'
import { useDeploySafety, useSetDeploySafety } from '../queries/deployGate'
import type { DeploySafety, DeploySafetyOverrides } from '../types/deployGate'
import {
  GATE_FIELDS,
  defaultText,
  toDraft,
  toOverrides,
} from '../lib/deploySafetyForm'
import { CardSkeleton } from '@/components/ui/card-skeleton'
import { RollbackTargetsList } from './RollbackTargetsList'

/** DeploySafetyCard edits one app's readiness gate, drain and post-deploy watch thresholds. */
export function DeploySafetyCard({ appName }: { appName: string }) {
  const { t } = useTranslation('deploySafety')
  const { data, isError, isPending } = useDeploySafety(appName)

  // Hold the card's height until the first response instead of an empty body.
  if (isPending) return <CardSkeleton titleWidth="w-48" rows={4} />

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ShieldCheckGlassIcon className="size-5 text-muted-foreground" />
          {t('settings.title')}
          <InfoTip label={t('settings.infoTipLabel')}>
            {t('settings.description')}
          </InfoTip>
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {isError ? (
          <p className="text-sm text-muted-foreground">
            {t('settings.unavailable')}
          </p>
        ) : null}
        {data ? (
          <SafetyForm
            key={JSON.stringify(data.overrides)}
            appName={appName}
            safety={data}
          />
        ) : null}
        <RollbackTargetsList appName={appName} />
      </CardContent>
    </Card>
  )
}

// Keyed on the saved overrides, so a save or refetch resets the draft.
function SafetyForm({
  appName,
  safety,
}: {
  appName: string
  safety: DeploySafety
}) {
  const { t } = useTranslation('deploySafety')
  const save = useSetDeploySafety(appName)
  const [draft, setDraft] = useState(() => toDraft(safety.overrides))

  function submit(next: DeploySafetyOverrides) {
    save.mutate(next, {
      onSuccess: () =>
        toast.add({ title: t('settings.toast.saved'), type: 'success' }),
      onError: (error) =>
        toast.add({
          title: t('settings.toast.error'),
          description: error.message,
          type: 'error',
        }),
    })
  }

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault()
        submit(toOverrides(draft, safety.overrides, GATE_FIELDS))
      }}
    >
      <p className="text-sm text-muted-foreground">
        {t('settings.inheritHint')}
      </p>
      <div className="grid gap-4 sm:grid-cols-2">
        {GATE_FIELDS.map(({ key, numeric }) => (
          <Field key={key}>
            <FieldLabel htmlFor={`deploy-safety-${key}`}>
              {t(`settings.field.${key}.label`)}
            </FieldLabel>
            <Input
              id={`deploy-safety-${key}`}
              inputMode={numeric ? 'decimal' : 'text'}
              value={draft[key]}
              placeholder={defaultText(key, safety.defaults)}
              onChange={(e) => setDraft({ ...draft, [key]: e.target.value })}
            />
            <FieldDescription className="text-xs">
              {t(`settings.field.${key}.hint`)}
            </FieldDescription>
          </Field>
        ))}
      </div>
      <div className="flex flex-wrap gap-2">
        <Button type="submit" size="sm" disabled={save.isPending}>
          {t('settings.save')}
        </Button>
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={save.isPending}
          onClick={() =>
            submit({
              ...safety.overrides,
              success_threshold: null,
              stable_window: null,
              drain_timeout: null,
            })
          }
        >
          {t('settings.reset')}
        </Button>
      </div>
    </form>
  )
}
