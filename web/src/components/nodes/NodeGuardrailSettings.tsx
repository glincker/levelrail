import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { toast } from '@/components/ui/toast'
import { useSaveGuardrails } from '../../queries/nodeCapacity'
import type {
  NodeCapacity,
  NodeGuardrailSettings as Settings,
} from '../../types/nodeCapacity'

import { FIELD_LABEL } from './capacityKeys'

type NumericKey = Exclude<keyof Settings, 'auto_clean'>

const FIELDS: { key: NumericKey; label: keyof typeof FIELD_LABEL }[] = [
  { key: 'disk_warn_free_pct', label: 'diskWarn' },
  { key: 'disk_crit_free_pct', label: 'diskCrit' },
  { key: 'inode_warn_free_pct', label: 'inodeWarn' },
  { key: 'inode_crit_free_pct', label: 'inodeCrit' },
  { key: 'mem_warn_avail_pct', label: 'memWarn' },
  { key: 'mem_crit_avail_pct', label: 'memCrit' },
  { key: 'swap_warn_used_pct', label: 'swapWarn' },
  { key: 'swap_crit_used_pct', label: 'swapCrit' },
]

function toText(v: number | null): string {
  return v === null ? '' : String(v)
}

export function NodeGuardrailSettings({
  nodeId,
  capacity,
}: Readonly<{ nodeId: string; capacity: NodeCapacity }>) {
  const { t } = useTranslation('nodes')
  const save = useSaveGuardrails(nodeId)
  const [text, setText] = useState<Record<NumericKey, string>>(
    () =>
      Object.fromEntries(
        FIELDS.map((f) => [f.key, toText(capacity.settings[f.key])]),
      ) as Record<NumericKey, string>,
  )
  const [autoClean, setAutoClean] = useState(capacity.settings.auto_clean)

  function submit() {
    const body = { auto_clean: autoClean } as Settings
    for (const f of FIELDS) {
      const raw = text[f.key].trim()
      body[f.key] = raw === '' ? null : Number(raw)
    }
    save.mutate(body, {
      onSuccess: () =>
        toast.add({ title: t('capacity.saved'), type: 'success' }),
    })
  }

  return (
    <div className="space-y-4">
      <p className="text-sm text-muted-foreground">
        {t('capacity.settingsHint')}
      </p>
      <div className="grid gap-3 sm:grid-cols-2">
        {FIELDS.map((f) => (
          <div key={f.key} className="space-y-1">
            <Label htmlFor={`guard-${f.key}`}>{t(FIELD_LABEL[f.label])}</Label>
            <Input
              id={`guard-${f.key}`}
              type="number"
              min={0}
              max={100}
              inputMode="decimal"
              placeholder={t('capacity.useDefault')}
              value={text[f.key]}
              onChange={(e) =>
                setText((prev) => ({ ...prev, [f.key]: e.target.value }))
              }
            />
          </div>
        ))}
      </div>
      <div className="flex items-start gap-3">
        <Switch
          id="guard-auto-clean"
          checked={autoClean}
          disabled={!capacity.cleanup_supported}
          onCheckedChange={setAutoClean}
        />
        <div>
          <Label htmlFor="guard-auto-clean">{t('capacity.autoClean')}</Label>
          <p className="text-xs text-muted-foreground">
            {capacity.cleanup_supported
              ? t('capacity.autoCleanHint')
              : t('capacity.cleanupUnsupported')}
          </p>
        </div>
      </div>
      {save.isError ? (
        <Alert variant="destructive">
          <AlertDescription>{save.error.message}</AlertDescription>
        </Alert>
      ) : null}
      <Button onClick={submit} disabled={save.isPending}>
        {save.isPending ? t('capacity.saving') : t('capacity.save')}
      </Button>
    </div>
  )
}
