import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  CircleAlertGlassIcon,
  PlugGlassIcon,
  RefreshCwGlassIcon,
  SearchGlassIcon,
  SpreadsheetGlassIcon,
  TerminalGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { CopyFailureDetail } from '@/components/attention/CopyFailureDetail'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import { formatSize } from '../../../lib/format'
import type { ExplorerErrorInfo } from '../../../lib/explorerError'
import type { DataCopyInfo } from '../../../queries/databaseDataCopy'
import type { DbEvidence } from '../../../types/databaseViewer'

const COPY_STATUS_TONE: Record<string, string> = {
  verified: 'text-tone-success',
  failed: 'text-tone-danger',
  copying: 'text-tone-warning',
}

function Panel({
  icon,
  title,
  tone = 'neutral',
  children,
}: {
  icon: React.ReactNode
  title: string
  tone?: 'neutral' | 'danger' | 'warning'
  children: React.ReactNode
}) {
  return (
    <div
      className={cn(
        'space-y-3 rounded-xl border p-6',
        tone === 'danger' && 'border-tone-danger-border bg-tone-danger-soft',
        tone === 'warning' && 'border-tone-warning-border bg-tone-warning-soft',
        tone === 'neutral' && 'border-border bg-card',
      )}
    >
      <div className="flex items-center gap-2">
        <span className="[&_svg]:size-5" aria-hidden="true">
          {icon}
        </span>
        <h2 className="text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight)">
          {title}
        </h2>
      </div>
      {children}
    </div>
  )
}

export function ExplorerLoading() {
  const { t } = useTranslation('databases')
  return (
    <div className="space-y-2" role="status" aria-live="polite">
      <Skeleton className="h-64 w-full" />
      <p className="text-xs text-muted-foreground">
        {t('viewer.states.loading')}
      </p>
    </div>
  )
}

export function ExplorerReadError({
  info,
  onRetry,
  retrying,
}: {
  info: ExplorerErrorInfo
  onRetry: () => void
  retrying: boolean
}) {
  const { t } = useTranslation('databases')
  return (
    <div role="alert">
      <Panel
        icon={<PlugGlassIcon size="1.2em" />}
        title={t('viewer.states.errorTitle')}
        tone="danger"
      >
        <p className="text-sm">{t(`viewer.states.reason.${info.reason}`)}</p>
        <p className="text-sm text-muted-foreground">
          {t('viewer.states.errorNote')}
        </p>
        <div className="flex flex-wrap items-center gap-3">
          <Button type="button" size="sm" disabled={retrying} onClick={onRetry}>
            <RefreshCwGlassIcon className="size-4" aria-hidden="true" />
            {t('viewer.states.retry')}
          </Button>
        </div>
        <details className="text-xs">
          <summary className="cursor-pointer text-muted-foreground">
            {t('viewer.states.details')}
          </summary>
          <pre className="mt-2 max-h-48 overflow-auto whitespace-pre-wrap break-all rounded-md bg-muted p-2 font-mono">
            {info.raw}
          </pre>
        </details>
      </Panel>
    </div>
  )
}

function CopyBlock({ copy }: { copy: DataCopyInfo }) {
  const { t } = useTranslation('databases')
  return (
    <div className="space-y-1 rounded-lg border border-border bg-background p-3 text-sm">
      <p>{t('viewer.states.migrationTarget')}</p>
      <p>
        {t('viewer.states.copyStatus')}{' '}
        <span className={cn('font-medium', COPY_STATUS_TONE[copy.status])}>
          {t(`viewer.states.copy.${copy.status}`)}
        </span>
      </p>
      {copy.failure ? (
        <>
          <p className="text-xs text-muted-foreground">
            {copy.failure.summary}
          </p>
          <CopyFailureDetail
            hint={copy.failure.hint}
            technical={copy.failure.technical}
          />
        </>
      ) : copy.reason ? (
        <p className="text-xs text-muted-foreground">{copy.reason}</p>
      ) : null}
      <p className="text-xs text-muted-foreground">
        <Link
          to="/settings/import-platform"
          className="text-primary hover:underline"
        >
          {t('viewer.states.openMigrationHub')}
        </Link>{' '}
        {t('viewer.states.copyCli')}
      </p>
    </div>
  )
}

export function ExplorerEmpty({
  databaseName,
  evidence,
  checkedAt,
  copy,
}: {
  databaseName: string
  evidence?: DbEvidence
  checkedAt?: string
  copy?: DataCopyInfo | null
}) {
  const { t } = useTranslation('databases')
  const isTarget =
    copy !== undefined &&
    copy !== null &&
    ['copying', 'verified', 'failed'].includes(copy.status)
  const verifiedButEmpty = isTarget && copy.status === 'verified'
  const contradiction = evidence !== undefined && evidence.user_tables > 0
  const hasWarning = verifiedButEmpty || contradiction
  const sizeKnown = evidence !== undefined
  return (
    <Panel
      icon={
        hasWarning ? (
          <CircleAlertGlassIcon className="text-tone-danger" size="1.2em" />
        ) : (
          <SpreadsheetGlassIcon size="1.2em" />
        )
      }
      title={
        isTarget || hasWarning
          ? t('viewer.states.emptyVerifyTitle')
          : t('viewer.states.emptyTitle')
      }
      tone={hasWarning ? 'warning' : 'neutral'}
    >
      {isTarget ? (
        <CopyBlock copy={copy} />
      ) : (
        <p className="text-sm text-muted-foreground">
          {t('viewer.states.emptyBody')}
        </p>
      )}
      {verifiedButEmpty ? (
        <p className="text-sm font-medium text-tone-warning" role="alert">
          {t('viewer.states.verifiedEmpty')}
        </p>
      ) : null}
      {contradiction ? (
        <p className="text-sm font-medium text-tone-warning" role="alert">
          {t('viewer.states.contradiction', { count: evidence.user_tables })}
        </p>
      ) : null}
      <dl className="grid gap-x-6 gap-y-1 text-sm sm:grid-cols-2">
        <dt className="text-muted-foreground">
          {t('viewer.states.evidence.size')}
        </dt>
        <dd className="tabular-nums">
          {sizeKnown ? formatSize(evidence.database_size_bytes) : '-'}
        </dd>
        <dt className="text-muted-foreground">
          {t('viewer.states.evidence.schemas')}
        </dt>
        <dd className="tabular-nums">
          {sizeKnown ? evidence.schemas_scanned : '-'}
        </dd>
        <dt className="text-muted-foreground">
          {t('viewer.states.evidence.tables')}
        </dt>
        <dd className="tabular-nums">
          {sizeKnown ? evidence.user_tables : '-'}
        </dd>
        <dt className="text-muted-foreground">
          {t('viewer.states.evidence.excluded')}
        </dt>
        <dd className="font-mono text-xs">
          {sizeKnown ? evidence.excluded_schemas.join(', ') : '-'}
        </dd>
        <dt className="text-muted-foreground">
          {t('viewer.states.evidence.checked')}
        </dt>
        <dd>{checkedAt ? new Date(checkedAt).toLocaleString() : '-'}</dd>
      </dl>
      {!sizeKnown ? (
        <p className="text-xs text-muted-foreground">
          {t('viewer.states.evidence.unavailable')}
        </p>
      ) : null}
      {isTarget ? null : (
        <Link
          to="/databases/$name/console"
          params={{ name: databaseName }}
          className="inline-flex items-center gap-1.5 text-sm font-medium text-primary hover:underline"
        >
          <TerminalGlassIcon className="size-4.5" aria-hidden="true" />
          {t('viewer.states.openConsole')}
        </Link>
      )}
    </Panel>
  )
}

export function ExplorerNoMatches({
  query,
  onClear,
}: {
  query: string
  onClear: () => void
}) {
  const { t } = useTranslation('databases')
  return (
    <div className="space-y-2 p-4 text-center">
      <SearchGlassIcon
        className="mx-auto size-5.5 text-muted-foreground"
        aria-hidden="true"
      />
      <p className="text-sm">{t('viewer.states.noMatches', { query })}</p>
      <p className="text-xs text-muted-foreground">
        {t('viewer.states.noMatchesNote')}
      </p>
      <Button type="button" size="sm" variant="outline" onClick={onClear}>
        {t('viewer.states.clearSearch')}
      </Button>
    </div>
  )
}
