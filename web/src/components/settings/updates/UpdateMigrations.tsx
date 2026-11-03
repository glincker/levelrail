import { useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { CircleAlertGlassIcon, CircleCheckGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { ApiError } from '@/lib/apiError'
import {
  selfUpgradePlanQueryOptions,
  selfUpgradeStatusQueryOptions,
  useMigrationDryRun,
  type PendingMigration,
} from '@/queries/selfUpgrade'

// Rows shown before the "Show all" disclosure.
export const MIGRATION_PREVIEW_COUNT = 5

const UNSUPPORTED = new Set([403, 404, 501])

// UpdateMigrations lists the pending database migrations and offers a dry
// run on a copy, so a risky migration is found before the upgrade starts.
export function UpdateMigrations() {
  const { t } = useTranslation('updates')
  const plan = useQuery(selfUpgradePlanQueryOptions(''))
  const status = useQuery(selfUpgradeStatusQueryOptions())
  const dry = useMigrationDryRun()
  const [showAll, setShowAll] = useState(false)
  const listRef = useRef<HTMLUListElement>(null)

  const list: PendingMigration[] = plan.data?.pending_migration_list ?? []
  const count = plan.data?.pending_migrations ?? list.length
  if (!plan.data || (count === 0 && list.length === 0)) {
    return plan.data && plan.data.pending_migrations === 0 ? (
      <p className="text-sm text-muted-foreground">
        {t('changes.migrations.none')}
      </p>
    ) : null
  }

  const err = dry.error instanceof ApiError ? dry.error : null
  const unsupported = err !== null && UNSUPPORTED.has(err.status)
  const running = status.data?.running != null
  const result = dry.data
  const appliedVersions = new Set(result?.applied.map((m) => m.version))
  const failing =
    result && !result.ok
      ? list.find((m) => !appliedVersions.has(m.version))
      : undefined
  const visible =
    showAll || failing !== undefined
      ? list
      : list.slice(0, MIGRATION_PREVIEW_COUNT)

  let errorText = ''
  if (err && !unsupported) {
    if (err.status === 429) errorText = t('changes.migrations.busy')
    else if (err.status === 507) errorText = t('changes.migrations.noSpace')
    else errorText = t('changes.migrations.error', { error: err.message })
  }

  return (
    <div className="space-y-2">
      <h4 className="text-xs text-muted-foreground">
        {t('changes.migrations.title')}
      </h4>
      <p className="text-sm">{t('changes.migrations.count', { count })}</p>
      {list.length > 0 ? (
        <>
          <ul ref={listRef} className="space-y-0.5 text-sm">
            {visible.map((m) => (
              <li
                key={m.version}
                data-failing={m.version === failing?.version}
                className="rounded px-1.5 py-0.5 text-muted-foreground data-[failing=true]:bg-destructive/10 data-[failing=true]:text-destructive"
              >
                {t('changes.migrations.item', {
                  version: m.version,
                  name: m.name,
                })}
              </li>
            ))}
          </ul>
          {list.length > MIGRATION_PREVIEW_COUNT && failing === undefined ? (
            <button
              type="button"
              aria-expanded={showAll}
              className="text-sm text-muted-foreground hover:text-foreground"
              onClick={() => {
                setShowAll((v) => !v)
              }}
            >
              {showAll
                ? t('changes.migrations.showLess')
                : t('changes.migrations.showAll', { count: list.length })}
            </button>
          ) : null}
        </>
      ) : null}
      {unsupported ? null : (
        <div className="flex flex-wrap items-center gap-2">
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={dry.isPending || running}
            onClick={() => {
              dry.mutate()
            }}
          >
            {dry.isPending
              ? t('changes.migrations.testing')
              : result || err
                ? t('changes.migrations.retry')
                : t('changes.migrations.test')}
          </Button>
        </div>
      )}
      {errorText !== '' ? (
        <p role="alert" className="text-sm text-destructive">
          {errorText}
        </p>
      ) : null}
      {result?.ok ? (
        <div
          role="status"
          className="flex items-start gap-2 rounded-md bg-tone-success-soft p-2 text-sm"
        >
          <CircleCheckGlassIcon
            aria-hidden
            className="mt-0.5 size-5 shrink-0 text-tone-success"
          />
          <div>
            <p className="font-medium">
              {t('changes.migrations.successTitle')}
            </p>
            <p>
              {t('changes.migrations.success', {
                count: result.applied.length,
              })}{' '}
              {t('changes.migrations.successTime', {
                seconds: (result.duration_ms / 1000).toFixed(1),
              })}
            </p>
          </div>
        </div>
      ) : null}
      {result && !result.ok ? (
        <div
          role="alert"
          className="flex items-start gap-2 rounded-md bg-tone-danger-soft p-2 text-sm"
        >
          <CircleAlertGlassIcon
            aria-hidden
            className="mt-0.5 size-5 shrink-0 text-tone-danger"
          />
          <div className="space-y-1">
            <p className="font-medium text-tone-danger">
              {t('changes.migrations.failTitle')}
            </p>
            <p className="break-words">
              {failing
                ? t('changes.migrations.failed', {
                    version: failing.version,
                    name: failing.name,
                    error: result.error ?? '',
                  })
                : t('changes.migrations.failedUnknown', {
                    error: result.error ?? '',
                  })}
            </p>
            <p>{t('changes.migrations.failHint')}</p>
            {failing ? (
              <button
                type="button"
                className="text-brand underline-offset-4 hover:underline"
                onClick={() => {
                  listRef.current
                    ?.querySelector('[data-failing="true"]')
                    ?.scrollIntoView({ block: 'nearest' })
                }}
              >
                {t('changes.migrations.viewFailing')}
              </button>
            ) : null}
          </div>
        </div>
      ) : null}
    </div>
  )
}
