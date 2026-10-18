import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { useIsRoot } from '../../hooks/useIsRoot'
import {
  JANITOR_STAGES,
  useSaveJanitorPolicy,
  type JanitorPolicy,
  type JanitorStageName,
} from '../../queries/janitor'

type NumberKey =
  | 'interval_minutes'
  | 'disk_free_budget_percent'
  | 'build_cache_budget_mb'
  | 'build_cache_max_age_hours'
  | 'volume_quarantine_days'
  | 'attempt_retention_days'
  | 'attempt_keep_per_app'

type PolicyLabel =
  | 'policy.interval'
  | 'policy.budget'
  | 'policy.cacheBudget'
  | 'policy.cacheAge'
  | 'policy.quarantineDays'
  | 'policy.attemptDays'
  | 'policy.attemptKeep'

type PolicyHelp = 'policy.budgetHelp' | 'policy.cacheBudgetHelp'

const NUMBER_FIELDS: {
  key: NumberKey
  label: PolicyLabel
  help?: PolicyHelp
  min: number
}[] = [
  { key: 'interval_minutes', label: 'policy.interval', min: 5 },
  {
    key: 'disk_free_budget_percent',
    label: 'policy.budget',
    help: 'policy.budgetHelp',
    min: 1,
  },
  {
    key: 'build_cache_budget_mb',
    label: 'policy.cacheBudget',
    help: 'policy.cacheBudgetHelp',
    min: 0,
  },
  { key: 'build_cache_max_age_hours', label: 'policy.cacheAge', min: 1 },
  { key: 'volume_quarantine_days', label: 'policy.quarantineDays', min: 1 },
  { key: 'attempt_retention_days', label: 'policy.attemptDays', min: 1 },
  { key: 'attempt_keep_per_app', label: 'policy.attemptKeep', min: 1 },
]

export function JanitorPolicyCard({ policy }: { policy: JanitorPolicy }) {
  const { t } = useTranslation('janitor')
  const isRoot = useIsRoot()
  const save = useSaveJanitorPolicy()
  const [draft, setDraft] = useState<JanitorPolicy>(policy)

  const valid = NUMBER_FIELDS.every(
    (f) => Number.isFinite(draft[f.key]) && draft[f.key] >= f.min,
  )

  function toggleStage(stage: JanitorStageName, on: boolean) {
    const rest = draft.disabled_stages.filter((s) => s !== stage)
    setDraft({ ...draft, disabled_stages: on ? rest : [...rest, stage] })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('policy.title')}</CardTitle>
      </CardHeader>
      <CardContent>
        <form
          className="space-y-5"
          onSubmit={(e) => {
            e.preventDefault()
            if (!valid) return
            save.mutate(draft, {
              onSuccess: () =>
                toast.add({ title: t('toast.saved'), type: 'success' }),
              onError: (err) =>
                toast.add({ title: err.message, type: 'error' }),
            })
          }}
        >
          <div className="grid gap-4 sm:grid-cols-3">
            <Field orientation="horizontal">
              <Switch
                id="janitor-enabled"
                checked={draft.enabled}
                disabled={!isRoot}
                onCheckedChange={(v) => setDraft({ ...draft, enabled: v })}
              />
              <div>
                <FieldLabel htmlFor="janitor-enabled">
                  {t('policy.enabled')}
                </FieldLabel>
                <FieldDescription>{t('policy.enabledHelp')}</FieldDescription>
              </div>
            </Field>
            <Field orientation="horizontal">
              <Switch
                id="janitor-dry"
                checked={draft.dry_run}
                disabled={!isRoot}
                onCheckedChange={(v) => setDraft({ ...draft, dry_run: v })}
              />
              <div>
                <FieldLabel htmlFor="janitor-dry">
                  {t('policy.dryRun')}
                </FieldLabel>
                <FieldDescription>{t('policy.dryRunHelp')}</FieldDescription>
              </div>
            </Field>
            <Field orientation="horizontal">
              <Switch
                id="janitor-delete-volumes"
                checked={draft.delete_orphan_volumes}
                disabled={!isRoot}
                onCheckedChange={(v) =>
                  setDraft({ ...draft, delete_orphan_volumes: v })
                }
              />
              <div>
                <FieldLabel htmlFor="janitor-delete-volumes">
                  {t('policy.deleteVolumes')}
                </FieldLabel>
                <FieldDescription>
                  {t('policy.deleteVolumesHelp')}
                </FieldDescription>
              </div>
            </Field>
          </div>

          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {NUMBER_FIELDS.map((f) => (
              <Field key={f.key}>
                <FieldLabel htmlFor={`janitor-${f.key}`}>
                  {t(f.label)}
                </FieldLabel>
                <Input
                  id={`janitor-${f.key}`}
                  type="number"
                  min={f.min}
                  step={1}
                  inputMode="numeric"
                  value={Number.isNaN(draft[f.key]) ? '' : draft[f.key]}
                  disabled={!isRoot}
                  onChange={(e) =>
                    setDraft({ ...draft, [f.key]: e.target.valueAsNumber })
                  }
                />
                {f.help ? (
                  <FieldDescription>{t(f.help)}</FieldDescription>
                ) : null}
              </Field>
            ))}
          </div>

          <fieldset className="space-y-2">
            <legend className="text-sm font-medium text-foreground">
              {t('policy.stages')}
            </legend>
            {JANITOR_STAGES.map((stage) => (
              <Field key={stage} orientation="horizontal">
                <Checkbox
                  id={`janitor-stage-${stage}`}
                  checked={!draft.disabled_stages.includes(stage)}
                  disabled={!isRoot}
                  onCheckedChange={(v) => toggleStage(stage, v === true)}
                />
                <div>
                  <FieldLabel htmlFor={`janitor-stage-${stage}`}>
                    {t(`stage.${stage}`)}
                  </FieldLabel>
                  <FieldDescription>{t(`stageHelp.${stage}`)}</FieldDescription>
                </div>
              </Field>
            ))}
          </fieldset>

          {!valid ? (
            <p role="alert" className="text-sm text-destructive">
              {t('policy.invalid')}
            </p>
          ) : null}
          {!isRoot ? (
            <p className="text-sm text-muted-foreground">
              {t('policy.rootOnly')}
            </p>
          ) : null}
          <Button type="submit" disabled={!isRoot || !valid || save.isPending}>
            {t('policy.save')}
          </Button>
        </form>
      </CardContent>
    </Card>
  )
}
