import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ChevronDownGlassIcon,
  ChevronRightGlassIcon,
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  InfoGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { Checkbox } from '../ui/checkbox'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '../ui/collapsible'
import { CommandBlock } from '../ui/code-block'
import { Skeleton } from '../ui/skeleton'
import { Button } from '../ui/button'
import { formatSize } from '../../lib/format'
import type { SelfUpgradePlan } from '../../queries/selfUpgrade'
import type { BlockReason } from './upgradeBlockReason'

export function UpgradeSafetySkeleton() {
  const { t } = useTranslation('upgradeSafety')
  return (
    <div
      role="status"
      aria-label={t('loading')}
      className="space-y-2 rounded-lg border border-border p-3"
    >
      <Skeleton className="h-4 w-40" />
      <Skeleton className="h-4 w-full max-w-md" />
      <Skeleton className="h-4 w-full max-w-sm" />
    </div>
  )
}

export function UpgradeSafetyError({
  message,
  onRetry,
}: {
  message: string
  onRetry: () => void
}) {
  const { t } = useTranslation('upgradeSafety')
  return (
    <div
      role="alert"
      className="flex flex-wrap items-center gap-x-3 gap-y-1 rounded-lg border border-tone-danger-border bg-tone-danger-soft p-3 text-sm"
    >
      <CircleAlertGlassIcon aria-hidden className="size-5 text-tone-danger" />
      <span className="min-w-0 flex-1">
        {t('loadError', { message })} {t('loadErrorNext')}
      </span>
      <Button type="button" size="sm" variant="outline" onClick={onRetry}>
        {t('retry')}
      </Button>
    </div>
  )
}

function Row({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <div className="grid gap-x-3 gap-y-0.5 sm:grid-cols-[7rem_1fr]">
      <dt className="text-xs font-medium text-muted-foreground">{label}</dt>
      <dd className="min-w-0 space-y-0.5 text-sm">{children}</dd>
    </div>
  )
}

interface Props {
  plan: SelfUpgradePlan
  skipBackup: boolean
  onSkipBackupChange: (v: boolean) => void
  // Set when the last start was refused because the backup failed.
  backupFailed: boolean
  // Preflight count, used when the plan could not read the schema.
  fallbackMigrations?: number
}

// UpgradeSafetyCard shows what one Upgrade click changes and protects.
export function UpgradeSafetyCard({
  plan,
  skipBackup,
  onSkipBackupChange,
  backupFailed,
  fallbackMigrations,
}: Props) {
  const { t } = useTranslation('upgradeSafety')
  const [open, setOpen] = useState(false)
  const expanded = open || backupFailed
  const disk = plan.disk
  const backup = plan.backup
  const migrations = plan.pending_migrations ?? fallbackMigrations
  const restore = backup?.restore_command
  const breaking = plan.breaking.length

  return (
    <section
      aria-label={t('title')}
      className="divide-y divide-border rounded-lg border border-border bg-card [&>*]:px-3 [&>*]:py-2.5"
    >
      {backupFailed ? (
        <div
          role="alert"
          className="flex items-start gap-2 bg-tone-danger-soft text-sm"
        >
          <CircleAlertGlassIcon
            aria-hidden
            className="mt-0.5 size-5 shrink-0 text-tone-danger"
          />
          <div className="space-y-0.5">
            <p className="font-semibold text-tone-danger">
              {t('backupFailed.title')}
            </p>
            <p className="text-foreground">{t('backupFailed.next')}</p>
          </div>
        </div>
      ) : null}
      <dl className="space-y-2">
        <Row label={t('change.label')}>
          <p className="font-medium">
            {t('change.fromTo', {
              from: plan.current_version,
              to: plan.target_version,
            })}
          </p>
          <p className="text-muted-foreground">
            {typeof migrations === 'number'
              ? migrations > 0
                ? t('change.migrations', { count: migrations })
                : t('change.noMigrations')
              : null}
            {plan.db_schema_version !== undefined &&
            plan.target_schema_version !== undefined &&
            migrations
              ? ` ${t('change.schemaNote', {
                  schema: t('change.schema', {
                    from: plan.db_schema_version,
                    to: plan.target_schema_version,
                  }),
                })}`
              : null}
          </p>
          {breaking > 0 ? (
            <p className="flex items-start gap-1.5 font-semibold text-tone-warning">
              <TriangleAlertGlassIcon
                aria-hidden
                className="mt-0.5 size-4 shrink-0"
              />
              {t('change.breaking', { count: breaking })}
            </p>
          ) : (
            <p className="text-muted-foreground">{t('change.noBreaking')}</p>
          )}
        </Row>
        <Row label={t('checks.label')}>
          {disk ? (
            <p className="flex items-start gap-1.5">
              {disk.known && disk.ok ? (
                <CircleCheckGlassIcon
                  aria-hidden
                  className="mt-0.5 size-4 shrink-0 text-tone-success"
                />
              ) : disk.known ? (
                <CircleAlertGlassIcon
                  aria-hidden
                  className="mt-0.5 size-4 shrink-0 text-tone-danger"
                />
              ) : (
                <InfoGlassIcon
                  aria-hidden
                  className="mt-0.5 size-4 shrink-0 text-muted-foreground"
                />
              )}
              <span
                className={disk.known ? undefined : 'text-muted-foreground'}
              >
                {!disk.known
                  ? t('checks.diskUnknown')
                  : t(disk.ok ? 'checks.diskOk' : 'checks.diskShort', {
                      free: formatSize(disk.free_bytes),
                      needed: formatSize(disk.required_bytes),
                    })}
              </span>
            </p>
          ) : null}
          {backup ? (
            <p className="text-muted-foreground">
              {skipBackup
                ? t('checks.backupSkipped')
                : backup.will_take && backup.dir
                  ? t('checks.backupAt', { dir: backup.dir })
                  : t('checks.backupNone')}
            </p>
          ) : null}
        </Row>
        {plan.downtime_seconds ? (
          <Row label={t('downtime.label')}>
            <p>{t('downtime.estimate', { seconds: plan.downtime_seconds })}</p>
            {plan.restart_behaviour ? (
              <p className="text-muted-foreground">
                {t('downtime.restart', { behaviour: plan.restart_behaviour })}
              </p>
            ) : null}
          </Row>
        ) : null}
      </dl>
      {restore && !skipBackup ? (
        <div className="space-y-1">
          <p className="text-xs font-medium text-muted-foreground">
            {t('restore.label')}
          </p>
          <CommandBlock code={restore} label={t('restore.copyLabel')} />
        </div>
      ) : null}
      {backup?.will_take ? (
        <Collapsible open={expanded} onOpenChange={setOpen}>
          <CollapsibleTrigger className="inline-flex w-auto items-center gap-1 rounded text-xs text-muted-foreground hover:text-foreground focus-visible:outline-2 focus-visible:outline-ring pointer-coarse:min-h-11">
            {expanded ? (
              <ChevronDownGlassIcon aria-hidden className="size-4" />
            ) : (
              <ChevronRightGlassIcon aria-hidden className="size-4" />
            )}
            {t('skip.toggle')}
          </CollapsibleTrigger>
          <CollapsiblePanel className="m-0 mt-2 space-y-1 p-0">
            <div className="flex items-center gap-2 pointer-coarse:min-h-11">
              <Checkbox
                id="skip-backup"
                checked={skipBackup}
                onCheckedChange={(v) => onSkipBackupChange(v === true)}
              />
              <label htmlFor="skip-backup" className="text-sm">
                {t('skip.label')}
              </label>
            </div>
            <p className="flex items-start gap-1.5 text-sm font-medium text-tone-warning">
              <TriangleAlertGlassIcon
                aria-hidden
                className="mt-0.5 size-4 shrink-0"
              />
              {backupFailed ? t('skip.hint') : t('skip.warning')}
            </p>
          </CollapsiblePanel>
        </Collapsible>
      ) : null}
    </section>
  )
}

// Shown after a successful upgrade so the restore point is not lost.
export function RestoreAfterUpgrade({ command }: { command: string }) {
  const { t } = useTranslation('upgradeSafety')
  return (
    <div className="space-y-1 rounded-lg bg-muted/40 p-3">
      <p className="text-xs font-medium text-muted-foreground">
        {t('restore.label')}
      </p>
      <CommandBlock code={command} label={t('restore.copyLabel')} />
      <p className="text-xs text-muted-foreground">{t('restore.after')}</p>
    </div>
  )
}

// Plain reason beside the disabled Upgrade button. A plan reason is already
// shown with its CLI fallback, so only the other kinds render here.
export function BlockReasonText({ block }: { block: BlockReason }) {
  const { t } = useTranslation('upgradeSafety')
  if (block.kind === 'plan' && block.reason) return null
  let text: string
  if (block.kind === 'disk') {
    text = t('blocked.disk', { short: formatSize(block.shortBytes) })
  } else if (block.kind === 'notes') {
    text = t('blocked.notes')
  } else {
    text = t('blocked.planUnknown')
  }
  return (
    <span role="status" className="text-xs text-tone-danger">
      {text}
    </span>
  )
}
