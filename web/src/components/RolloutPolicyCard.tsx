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
  const busy = save.isPending || reset.isPending

  function fail(error: Error) {
    toast.add({
      title: t('rolloutPolicy.saveFailed'),
      description: error.message,
      type: 'error',
    })
  }

  function onSave() {
    save.mutate(draft, {
      onSuccess: () =>
        toast.add({ title: t('rolloutPolicy.saved'), type: 'success' }),
      onError: fail,
    })
  }

  function onReset() {
    reset.mutate(undefined, {
      onSuccess: () => {
        setDraft({})
        toast.add({ title: t('rolloutPolicy.resetDone'), type: 'success' })
      },
      onError: fail,
    })
  }

  return (
    <CardContent className="space-y-3">
      <p className="text-sm text-muted-foreground">{t('rolloutPolicy.hint')}</p>
      <div className="flex flex-wrap items-end gap-3">
        {FIELDS.map((f) => (
          <div key={f} className="space-y-1">
            <Label htmlFor={`rollout-${f}`}>{t(`rolloutPolicy.${f}`)}</Label>
            <Input
              id={`rollout-${f}`}
              className="w-28"
              value={draft[f] ?? ''}
              placeholder={t('rolloutPolicy.default')}
              disabled={busy}
              onChange={(e) => setDraft({ ...draft, [f]: e.target.value })}
            />
          </div>
        ))}
        <Button type="button" size="sm" disabled={busy} onClick={onSave}>
          {t('rolloutPolicy.save')}
        </Button>
        <Button
          type="button"
          size="sm"
          variant="ghost"
          disabled={busy}
          onClick={onReset}
        >
          {t('rolloutPolicy.reset')}
        </Button>
      </div>
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
      <PolicyForm appName={appName} initial={data.policy ?? {}} />
    </Card>
  )
}
