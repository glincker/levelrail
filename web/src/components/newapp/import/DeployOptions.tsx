import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { PlusGlassIcon, TrashGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { CronPreview } from '../../CronPreview'
import {
  CRON_PRESETS,
  jobError,
  newJob,
  previewSupport,
  setJobPreset,
  type DeployOptionsValue,
  type JobDraft,
  type JobPreset,
} from '../../../lib/deployOptions'
import type { ImportAnalysis } from '../../../queries/importAnalyze'

const PRESET_IDS: JobPreset[] = [...CRON_PRESETS.map((p) => p.id), 'custom']

/** Deploy on push, PR previews, health check and scheduled jobs for an import. */
export function DeployOptions({
  analysis,
  branch,
  value,
  onChange,
  hidePush = false,
}: {
  analysis: ImportAnalysis
  branch: string
  value: DeployOptionsValue
  onChange: (next: DeployOptionsValue) => void
  hidePush?: boolean
}) {
  const { t } = useTranslation('newApp')
  const [jobsOpen, setJobsOpen] = useState(false)
  const [nextId, setNextId] = useState(1)
  const support = previewSupport(analysis.repo_url)
  const healthDefault = analysis.health.value || '/'

  function updateJob(id: number, next: JobDraft) {
    onChange({
      ...value,
      jobs: value.jobs.map((j) => (j.id === id ? next : j)),
    })
  }

  return (
    <fieldset className="min-w-0 space-y-4 rounded-md border border-border p-4">
      <legend className="px-1 text-sm font-medium text-foreground">
        {t('deployOptions.title')}
      </legend>

      {hidePush ? null : (
        <div className="flex min-w-0 items-start justify-between gap-3">
          <div className="min-w-0">
            <label
              htmlFor="opt-push"
              className="text-sm font-medium text-foreground"
            >
              {t('deployOptions.push.label', { branch: branch || 'main' })}
            </label>
            <p className="text-xs text-muted-foreground">
              {value.deployOnPush
                ? t('deployOptions.push.on')
                : t('deployOptions.push.off')}
            </p>
          </div>
          <Switch
            id="opt-push"
            checked={value.deployOnPush}
            onCheckedChange={(v) => {
              onChange({ ...value, deployOnPush: v })
            }}
          />
        </div>
      )}

      <div className="flex min-w-0 items-start justify-between gap-3">
        <div className="min-w-0">
          <label
            htmlFor="opt-previews"
            className="flex flex-wrap items-center gap-2 text-sm font-medium text-foreground"
          >
            {t('deployOptions.previews.label')}
            {support === 'beta' ? (
              <Badge variant="outline">{t('deployOptions.beta')}</Badge>
            ) : null}
          </label>
          <p className="text-xs text-muted-foreground">
            {support === 'none'
              ? t('deployOptions.previews.unsupported')
              : t('deployOptions.previews.hint')}
          </p>
        </div>
        <Switch
          id="opt-previews"
          disabled={support === 'none'}
          checked={value.previews && support !== 'none'}
          onCheckedChange={(v) => {
            onChange({ ...value, previews: v })
          }}
        />
      </div>

      <Field>
        <FieldLabel htmlFor="opt-health">
          {t('deployOptions.health.label')}
        </FieldLabel>
        <Input
          id="opt-health"
          value={value.healthPath}
          placeholder={healthDefault}
          autoComplete="off"
          spellCheck={false}
          onChange={(e) => {
            onChange({ ...value, healthPath: e.target.value })
          }}
        />
        <FieldDescription>{t('deployOptions.health.hint')}</FieldDescription>
      </Field>

      <Collapsible open={jobsOpen} onOpenChange={setJobsOpen}>
        <CollapsibleTrigger className="text-sm text-muted-foreground underline underline-offset-2">
          {t('deployOptions.jobs.title')}
        </CollapsibleTrigger>
        <CollapsiblePanel className="space-y-3 pt-3">
          <p className="text-xs text-muted-foreground">
            {t('deployOptions.jobs.hint')}
          </p>
          {value.jobs.map((job) => {
            const err = job.path.trim() === '' ? null : jobError(job)
            return (
              <div
                key={job.id}
                className="min-w-0 space-y-2 rounded-md border border-border p-3"
              >
                <div className="flex min-w-0 flex-col gap-2 sm:flex-row">
                  <Input
                    aria-label={t('deployOptions.jobs.path')}
                    placeholder="/api/cron/cleanup"
                    value={job.path}
                    aria-invalid={err === 'path'}
                    className="min-w-0 font-mono sm:flex-1"
                    onChange={(e) => {
                      updateJob(job.id, { ...job, path: e.target.value })
                    }}
                  />
                  <Select
                    value={job.preset}
                    onValueChange={(v) => {
                      if (v) updateJob(job.id, setJobPreset(job, v))
                    }}
                  >
                    <SelectTrigger
                      aria-label={t('deployOptions.jobs.schedule')}
                      className="w-full sm:w-48"
                    >
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {PRESET_IDS.map((p) => (
                        <SelectItem key={p} value={p}>
                          {t(`deployOptions.jobs.presets.${p}`)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <Button
                    type="button"
                    size="sm"
                    variant="ghost"
                    aria-label={t('deployOptions.jobs.remove')}
                    onClick={() => {
                      onChange({
                        ...value,
                        jobs: value.jobs.filter((j) => j.id !== job.id),
                      })
                    }}
                  >
                    <TrashGlassIcon aria-hidden="true" className="size-4.5" />
                  </Button>
                </div>
                {job.preset === 'custom' ? (
                  <Input
                    aria-label={t('deployOptions.jobs.cron')}
                    value={job.cron}
                    aria-invalid={err === 'cron'}
                    className="font-mono"
                    onChange={(e) => {
                      updateJob(job.id, { ...job, cron: e.target.value })
                    }}
                  />
                ) : null}
                {err === 'path' ? (
                  <p className="text-xs text-destructive">
                    {t('deployOptions.jobs.pathError')}
                  </p>
                ) : null}
                <CronPreview cron={job.cron} timezone="UTC" />
              </div>
            )
          })}
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => {
              onChange({ ...value, jobs: [...value.jobs, newJob(nextId)] })
              setNextId(nextId + 1)
            }}
          >
            <PlusGlassIcon aria-hidden="true" className="size-4.5" />
            {t('deployOptions.jobs.add')}
          </Button>
        </CollapsiblePanel>
      </Collapsible>
    </fieldset>
  )
}
