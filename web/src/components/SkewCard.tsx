import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import {
  useRuntimeSettings,
  useSetRuntimeSetting,
} from '../queries/runtimeSettings'
import { useIsRoot } from '../hooks/useIsRoot'

const KEY_ID = 'deploy.next_deployment_id'
const KEY_GRACE = 'deploy.next_asset_grace_seconds'
const KEY_HELD = 'deploy.next_asset_grace_max_held'

// Skew protection is a control plane runtime setting that applies to every Next.js app, not a per-app field.
export function SkewCard() {
  const { t } = useTranslation('resources', { keyPrefix: 'skew' })
  const isRoot = useIsRoot()
  const { data } = useRuntimeSettings()
  const save = useSetRuntimeSetting()
  const find = (key: string) => data?.settings.find((s) => s.key === key)
  const idOn = find(KEY_ID)
  const grace = find(KEY_GRACE)
  const held = find(KEY_HELD)
  const [draftGrace, setDraftGrace] = useState<string | null>(null)
  const [draftHeld, setDraftHeld] = useState<string | null>(null)
  if (!idOn || !grace || !held) return null

  const disabled = !isRoot || save.isPending
  function write(key: string, value: boolean | number) {
    save.mutate(
      { key, value },
      {
        onSuccess: () => {
          toast.add({ title: t('saved'), type: 'success' })
        },
        onError: (e) => {
          toast.add({
            title: t('failed'),
            description: e.message,
            type: 'error',
          })
        },
      },
    )
  }
  const graceText = draftGrace ?? String(grace.value)
  const heldText = draftHeld ?? String(held.value)

  return (
    <section
      aria-label={t('title')}
      className="space-y-3 rounded-xl border border-border bg-card p-3 shadow-sm"
    >
      <div>
        <h3 className="text-sm font-semibold">{t('title')}</h3>
        <p className="text-xs text-muted-foreground">{t('hint')}</p>
      </div>
      <div className="flex items-center justify-between gap-3">
        <Label htmlFor="skew-id" className="text-sm">
          {t('deploymentId')}
        </Label>
        <Switch
          id="skew-id"
          checked={idOn.value === true}
          disabled={disabled}
          onCheckedChange={(v) => {
            write(KEY_ID, v)
          }}
        />
      </div>
      <div className="flex flex-wrap items-end gap-3">
        <div className="grid gap-1">
          <Label htmlFor="skew-grace" className="text-xs">
            {t('grace')}
          </Label>
          <Input
            id="skew-grace"
            type="number"
            min={0}
            className="w-28"
            value={graceText}
            disabled={disabled}
            onChange={(e) => {
              setDraftGrace(e.target.value)
            }}
          />
        </div>
        <div className="grid gap-1">
          <Label htmlFor="skew-held" className="text-xs">
            {t('held')}
          </Label>
          <Input
            id="skew-held"
            type="number"
            min={1}
            max={3}
            className="w-28"
            value={heldText}
            disabled={disabled}
            onChange={(e) => {
              setDraftHeld(e.target.value)
            }}
          />
        </div>
        <Button
          type="button"
          disabled={disabled || (draftGrace === null && draftHeld === null)}
          onClick={() => {
            if (draftGrace !== null) write(KEY_GRACE, Number(draftGrace))
            if (draftHeld !== null) write(KEY_HELD, Number(draftHeld))
            setDraftGrace(null)
            setDraftHeld(null)
          }}
        >
          {t('save')}
        </Button>
      </div>
      {isRoot ? null : (
        <p className="text-xs text-muted-foreground">{t('rootOnly')}</p>
      )}
    </section>
  )
}
