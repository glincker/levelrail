import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Field, FieldHint, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import {
  useSetVolumeBackupPolicy,
  useVolumeBackupPolicy,
} from '../../queries/backupProtection'
import type { VolumeBackupPolicy } from '../../types/backupProtection'

function toCount(value: string): number {
  const n = Number.parseInt(value, 10)
  return Number.isNaN(n) || n < 0 ? 0 : n
}

function PolicyForm({
  app,
  volume,
  initial,
  onDone,
}: {
  app: string
  volume: string
  initial: VolumePolicyDraft
  onDone: () => void
}) {
  const { t } = useTranslation('backups')
  const save = useSetVolumeBackupPolicy(app, volume)
  const [daily, setDaily] = useState(String(initial.retain_daily))
  const [weekly, setWeekly] = useState(String(initial.retain_weekly))
  const [monthly, setMonthly] = useState(String(initial.retain_monthly))
  const [preHook, setPreHook] = useState(initial.pre_hook)
  const [postHook, setPostHook] = useState(initial.post_hook)
  const [pause, setPause] = useState(initial.quiesce === 'pause')

  function submit() {
    save.mutate(
      {
        service_name: app,
        volume_name: volume,
        retain_daily: toCount(daily),
        retain_weekly: toCount(weekly),
        retain_monthly: toCount(monthly),
        pre_hook: preHook,
        post_hook: postHook,
        quiesce: pause ? 'pause' : '',
      },
      {
        onSuccess: () => {
          toast.add({ title: t('policy.saved'), type: 'success' })
          onDone()
        },
        onError: (err) => {
          toast.add({
            title: t('policy.failed'),
            description: err.message,
            type: 'error',
          })
        },
      },
    )
  }

  return (
    <>
      <div className="grid grid-cols-3 gap-3">
        {(
          [
            ['daily', daily, setDaily],
            ['weekly', weekly, setWeekly],
            ['monthly', monthly, setMonthly],
          ] as const
        ).map(([key, value, set]) => (
          <Field key={key}>
            <FieldLabel htmlFor={`policy-${key}`}>
              {t(`policy.${key}`)}
            </FieldLabel>
            <Input
              id={`policy-${key}`}
              type="number"
              min={0}
              inputMode="numeric"
              value={value}
              onChange={(e) => {
                set(e.target.value)
              }}
            />
          </Field>
        ))}
      </div>
      <FieldHint>{t('policy.retentionHint')}</FieldHint>
      <Field>
        <FieldLabel htmlFor="policy-pre">{t('policy.preHook')}</FieldLabel>
        <Input
          id="policy-pre"
          className="font-mono"
          autoComplete="off"
          spellCheck={false}
          value={preHook}
          onChange={(e) => {
            setPreHook(e.target.value)
          }}
        />
        <FieldHint>{t('policy.preHookHint')}</FieldHint>
      </Field>
      <Field>
        <FieldLabel htmlFor="policy-post">{t('policy.postHook')}</FieldLabel>
        <Input
          id="policy-post"
          className="font-mono"
          autoComplete="off"
          spellCheck={false}
          value={postHook}
          onChange={(e) => {
            setPostHook(e.target.value)
          }}
        />
      </Field>
      <div className="flex items-start gap-3">
        <Switch
          id="policy-pause"
          checked={pause}
          onCheckedChange={setPause}
          aria-describedby="policy-pause-hint"
        />
        <div className="space-y-1">
          <FieldLabel htmlFor="policy-pause">{t('policy.pause')}</FieldLabel>
          <p id="policy-pause-hint" className="text-xs text-muted-foreground">
            {t('policy.pauseHint')}
          </p>
        </div>
      </div>
      <DialogFooter>
        <Button type="button" disabled={save.isPending} onClick={submit}>
          {t('policy.save')}
        </Button>
      </DialogFooter>
    </>
  )
}

type VolumePolicyDraft = Omit<
  VolumeBackupPolicy,
  'service_name' | 'volume_name'
>

/** VolumePolicyDialog edits a volume's retention counts and backup hooks. */
export function VolumePolicyDialog({
  app,
  volume,
  open,
  onOpenChange,
}: {
  app: string
  volume: string
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation('backups')
  const policy = useVolumeBackupPolicy(app, volume, open)

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('policy.title', { app, volume })}</DialogTitle>
          <DialogDescription>{t('policy.description')}</DialogDescription>
        </DialogHeader>
        {policy.isError ? (
          <p className="text-sm text-destructive">
            {t('policy.loadFailed', { error: policy.error.message })}
          </p>
        ) : policy.data ? (
          <PolicyForm
            key={policy.dataUpdatedAt}
            app={app}
            volume={volume}
            initial={policy.data}
            onDone={() => {
              onOpenChange(false)
            }}
          />
        ) : null}
      </DialogContent>
    </Dialog>
  )
}
