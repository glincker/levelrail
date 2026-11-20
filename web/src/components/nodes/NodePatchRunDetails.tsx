import { useTranslation } from 'react-i18next'
import { Badge, type badgeVariants } from '@/components/ui/badge'
import { CodeBlock } from '@/components/ui/code-block'
import type { VariantProps } from 'class-variance-authority'
import type { PatchRun, PatchRunStatus } from '../../queries/nodePatch'

const STATUS_VARIANT: Record<
  PatchRunStatus,
  VariantProps<typeof badgeVariants>['variant']
> = {
  running: 'info',
  succeeded: 'success',
  failed: 'destructive',
  timed_out: 'warning',
}

export function PatchRunStatusBadge({ status }: { status: PatchRunStatus }) {
  const { t } = useTranslation('nodePatch')
  return (
    <Badge variant={STATUS_VARIANT[status] ?? 'muted'}>
      {t(`status.${status}`, { defaultValue: status })}
    </Badge>
  )
}

// Facts, notes and the output tail of one run; shared by history and the live dialog.
export function PatchRunDetails({ run }: { run: PatchRun }) {
  const { t } = useTranslation('nodePatch')
  const done = run.status !== 'running'
  return (
    <div className="space-y-2 text-xs text-muted-foreground">
      <p>
        {t('run.started', {
          time: new Date(run.started_at).toLocaleString(),
        })}
        {run.finished_at
          ? ` · ${t('run.finished', {
              time: new Date(run.finished_at).toLocaleString(),
            })}`
          : ''}
        {done ? ` · ${t('run.exitCode', { code: run.exit_code })}` : ''}
      </p>
      {run.packages.length > 0 ? (
        <p className="break-words font-mono">
          {t('run.packages', { names: run.packages.join(', ') })}
        </p>
      ) : null}
      {run.include_all ? <p>{t('run.withHeld')}</p> : null}
      {run.held.length > 0 ? (
        <p className="break-words font-mono">
          {t('run.held', { names: run.held.join(', ') })}
        </p>
      ) : null}
      {run.error ? (
        <p role="alert" className="text-tone-danger">
          {run.error}
        </p>
      ) : null}
      {run.reboot_scheduled ? <p>{t('run.rebootScheduled')}</p> : null}
      {run.reboot_required && !run.reboot_scheduled ? (
        <p>{t('run.rebootRequired')}</p>
      ) : null}
      {run.reboot_note ? <p>{run.reboot_note}</p> : null}
      {run.patch_status_after ? (
        <p className="text-foreground">
          {t('run.after', {
            total: run.patch_status_after.total,
            security: run.patch_status_after.security,
          })}
        </p>
      ) : null}
      <CodeBlock
        code={run.output_tail || t('dialog.noOutput')}
        label={t('run.output')}
        wrapToggle={false}
        maxHeight="md"
      />
    </div>
  )
}
