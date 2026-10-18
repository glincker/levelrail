import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { SettingRow, SettingRows } from '@/components/ui/setting-row'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { formatAge } from '../lib/format'
import { useSaveArchivePolicy } from '../queries/archives'
import type { ArchivePolicy } from '../queries/archives'

type DayField =
  'audit_hot_days' | 'deploy_hot_days' | 'pipeline_hot_days' | 'keep_days'

type LabelKey =
  | 'policy.auditDays'
  | 'policy.deployDays'
  | 'policy.pipelineDays'
  | 'policy.keepDays'

const DAY_FIELDS: { field: DayField; labelKey: LabelKey; min: number }[] = [
  { field: 'audit_hot_days', labelKey: 'policy.auditDays', min: 1 },
  { field: 'deploy_hot_days', labelKey: 'policy.deployDays', min: 1 },
  { field: 'pipeline_hot_days', labelKey: 'policy.pipelineDays', min: 1 },
  { field: 'keep_days', labelKey: 'policy.keepDays', min: 0 },
]

export function ArchivePolicyForm({ policy }: { policy: ArchivePolicy }) {
  const { t } = useTranslation('archives')
  const save = useSaveArchivePolicy()
  const [draft, setDraft] = useState<ArchivePolicy>(policy)

  return (
    <div className="space-y-4">
      <div>
        <h3 className="text-sm font-medium">{t('policy.title')}</h3>
        <p className="text-xs text-muted-foreground">
          {t('policy.description')}
        </p>
      </div>
      <SettingRows>
        <SettingRow
          title={t('policy.enabled')}
          description={t('policy.enabledHint')}
        >
          <Switch
            checked={draft.enabled}
            aria-label={t('policy.enabled')}
            onCheckedChange={(enabled) => {
              setDraft({ ...draft, enabled })
            }}
          />
        </SettingRow>
        {DAY_FIELDS.map(({ field, labelKey, min }) => (
          <SettingRow
            key={field}
            title={t(labelKey)}
            description={
              field === 'keep_days' ? t('policy.keepHint') : undefined
            }
          >
            <Label className="sr-only" htmlFor={`archive-${field}`}>
              {t(labelKey)}
            </Label>
            <Input
              id={`archive-${field}`}
              type="number"
              min={min}
              className="w-28"
              value={draft[field]}
              onChange={(e) => {
                setDraft({ ...draft, [field]: Number(e.target.value) })
              }}
            />
          </SettingRow>
        ))}
      </SettingRows>
      {policy.last_run_error ? (
        <Alert variant="destructive">
          <AlertDescription>
            {t('policy.lastRunFailed', { error: policy.last_run_error })}
          </AlertDescription>
        </Alert>
      ) : policy.last_run_at ? (
        <p className="text-xs text-muted-foreground">
          {t('policy.lastRun', { age: formatAge(policy.last_run_at) })}
        </p>
      ) : null}
      {save.isError ? (
        <p className="text-sm text-destructive">{save.error.message}</p>
      ) : null}
      <Button
        disabled={save.isPending}
        onClick={() => {
          save.mutate(draft, {
            onSuccess: () => {
              toast.add({ title: t('policy.saved'), type: 'success' })
            },
          })
        }}
      >
        {save.isPending ? t('policy.saving') : t('policy.save')}
      </Button>
    </div>
  )
}
