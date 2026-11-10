import { useState, type ReactNode } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  ArchiveGlassIcon,
  ArrowRightGlassIcon,
  CheckGlassIcon,
  ChevronDownGlassIcon,
  ChevronRightGlassIcon,
  ClockGlassIcon,
  ExternalLinkGlassIcon,
  InfoGlassIcon,
  RotateCcwGlassIcon,
  TriangleAlertGlassIcon,
  UserGlassIcon,
} from '@thesvg/ui-react'
import { StatusPill, type Tone } from '@/components/kit'
import { formatRelative } from '@/components/kit/formatRelative'
import { cn } from '@/lib/utils'
import { Alert, AlertDescription, AlertTitle } from '../ui/alert'
import { Button } from '../ui/button'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '../ui/collapsible'
import { EmptyState } from '../ui/empty-state'
import { Skeleton } from '../ui/skeleton'
import { toast } from '../ui/toast'
import {
  upgradeHistoryQueryOptions,
  useAckUpgrade,
  type UpgradeHistoryItem,
  type UpgradeKind,
} from '../../queries/upgradeHistory'
import { ReleaseNotes } from './ReleaseNotes'
import { RollbackPlanDialog } from './RollbackPlanDialog'
import { AgentVersionTimeline } from './AgentVersionTimeline'

// Rows shown before "Show all"; the dashboard keeps this fixed.
const PREVIEW_COUNT = 10

const KIND_TONE: Record<UpgradeKind, Tone> = {
  installed: 'info',
  adopted: 'neutral',
  upgraded: 'success',
  rolled_back: 'warning',
  rebuilt: 'neutral',
  development: 'neutral',
  changed: 'neutral',
}

function VersionChip({ value }: { value: string }) {
  return (
    <span className="rounded-full bg-muted px-2 py-0.5 font-mono text-xs text-foreground">
      {value}
    </span>
  )
}

function MetaChip({
  tone,
  icon,
  label,
}: {
  tone: 'muted' | 'warning'
  icon: ReactNode
  label: string
}) {
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1 text-xs',
        tone === 'warning' ? 'text-tone-warning' : 'text-muted-foreground',
      )}
    >
      {icon}
      {label}
    </span>
  )
}

function HistoryRow({
  item,
  onRollback,
  onAcked,
}: {
  item: UpgradeHistoryItem
  onRollback: (version: string) => void
  onAcked: (version: string) => void
}) {
  const { t } = useTranslation('updates')
  const ack = useAckUpgrade()
  const [open, setOpen] = useState(false)
  const hasNotes = item.notes_state === 'fetched' && item.notes !== ''
  const absolute = new Date(item.occurred_at).toLocaleString()
  const who =
    item.initiator === 'unknown'
      ? t('upgradeHistory.unknownShort')
      : item.method
        ? t('upgradeHistory.viaShort', {
            who: item.initiator,
            method: item.method,
          })
        : t('upgradeHistory.byShort', { who: item.initiator })
  const schemaKnown = item.schema_before !== null && item.schema_after !== null
  const backupMissing = item.backup_name === '' && item.schema_moved

  return (
    <li
      className={cn(
        'border-l-2 py-2 pl-3',
        item.acknowledged ? 'border-transparent' : 'border-primary',
      )}
    >
      <div className="flex flex-col gap-1.5 sm:flex-row sm:items-center sm:gap-3">
        <div className="flex min-w-0 flex-wrap items-center gap-2 sm:flex-1">
          <StatusPill
            tone={KIND_TONE[item.kind]}
            size="sm"
            label={t(`upgradeHistory.kind.${item.kind}`)}
          />
          {item.from_version !== '' ? (
            <>
              <VersionChip value={item.from_version} />
              <ArrowRightGlassIcon
                className="size-4 text-muted-foreground"
                aria-hidden="true"
              />
            </>
          ) : null}
          <VersionChip value={item.to_version} />
          <time
            dateTime={item.occurred_at}
            title={absolute}
            className="text-xs text-muted-foreground"
          >
            {formatRelative(item.occurred_at)}
          </time>
        </div>
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
          <MetaChip
            tone="muted"
            icon={<UserGlassIcon className="size-4" aria-hidden="true" />}
            label={who}
          />
          {item.schema_moved && schemaKnown ? (
            <span className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs text-muted-foreground">
              {t('upgradeHistory.schemaChip', {
                before: item.schema_before,
                after: item.schema_after,
              })}
            </span>
          ) : null}
          {item.backup_name !== '' ? (
            <MetaChip
              tone="muted"
              icon={<ArchiveGlassIcon className="size-4" aria-hidden="true" />}
              label={t('upgradeHistory.backupOk')}
            />
          ) : (
            <MetaChip
              tone={backupMissing ? 'warning' : 'muted'}
              icon={
                backupMissing ? (
                  <TriangleAlertGlassIcon
                    className="size-4"
                    aria-hidden="true"
                  />
                ) : (
                  <ArchiveGlassIcon className="size-4" aria-hidden="true" />
                )
              }
              label={
                backupMissing
                  ? t('upgradeHistory.backupMissing')
                  : t('upgradeHistory.backupNone')
              }
            />
          )}
          <span
            title={t(`upgradeHistory.health.${item.health}`, item.health)}
            className={cn(
              'size-2 shrink-0 rounded-full',
              item.health === 'booted'
                ? 'bg-tone-success'
                : 'bg-muted-foreground',
            )}
          >
            <span className="sr-only">
              {t(`upgradeHistory.health.${item.health}`, item.health)}
            </span>
          </span>
          {item.acknowledged ? (
            <span className="inline-flex items-center gap-1 text-xs text-muted-foreground">
              <CheckGlassIcon className="size-4" aria-hidden="true" />
              {t('upgradeHistory.acknowledged')}
            </span>
          ) : (
            <Button
              type="button"
              size="sm"
              className="min-h-10 sm:min-h-8"
              aria-label={t('upgradeHistory.ackAria', {
                version: item.to_version,
              })}
              disabled={ack.isPending}
              onClick={() => {
                ack.mutate(item.id, {
                  onSuccess: () => {
                    onAcked(item.to_version)
                    toast.add({
                      title: t('upgradeHistory.acknowledgedToast'),
                      type: 'success',
                    })
                  },
                  onError: (e) => {
                    toast.add({ title: e.message, type: 'error' })
                  },
                })
              }}
            >
              <CheckGlassIcon className="size-4" aria-hidden="true" />
              {t('upgradeHistory.acknowledge')}
            </Button>
          )}
          <button
            type="button"
            aria-expanded={open}
            aria-label={t('upgradeHistory.details', {
              version: item.to_version,
            })}
            className="inline-flex size-10 items-center justify-center rounded-md text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/60 sm:size-8"
            onClick={() => {
              setOpen((v) => !v)
            }}
          >
            {open ? (
              <ChevronDownGlassIcon className="size-4" aria-hidden="true" />
            ) : (
              <ChevronRightGlassIcon className="size-4" aria-hidden="true" />
            )}
          </button>
        </div>
      </div>

      {open ? (
        <div className="mt-2 space-y-2 pb-1">
          {item.schema_moved ? (
            <p className="text-xs text-tone-warning">
              {t('upgradeHistory.schemaMoved')}
            </p>
          ) : null}
          <div className="flex flex-wrap items-center gap-3">
            {item.release_url !== '' ? (
              <a
                href={item.release_url}
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex items-center gap-1 text-xs text-primary hover:underline"
              >
                {t('upgradeHistory.releasePage')}
                <ExternalLinkGlassIcon className="size-4" />
              </a>
            ) : item.kind === 'development' ? (
              <span className="text-xs text-muted-foreground">
                {t('upgradeHistory.noReleasePage')}
              </span>
            ) : null}
            {item.compare_url !== '' ? (
              <a
                href={item.compare_url}
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex items-center gap-1 text-xs text-primary hover:underline"
              >
                {t('upgradeHistory.compare')}
                <ExternalLinkGlassIcon className="size-4" />
              </a>
            ) : null}
            {item.rollback_available ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                className="min-h-10 sm:min-h-8"
                onClick={() => {
                  onRollback(item.from_version)
                }}
              >
                <RotateCcwGlassIcon className="size-4" />
                {t('upgradeHistory.rollbackTo', { version: item.from_version })}
              </Button>
            ) : null}
            {!hasNotes ? (
              <span className="text-xs text-muted-foreground">
                {item.notes_state === 'pending'
                  ? t('upgradeHistory.notesPending')
                  : t('upgradeHistory.notesUnavailable')}
              </span>
            ) : null}
          </div>
          {hasNotes ? <ReleaseNotes markdown={item.notes} /> : null}
          {item.acknowledged ? (
            <p className="text-xs text-muted-foreground">
              {t('upgradeHistory.ackedBy', {
                who: item.acked_by || t('upgradeHistory.initiatorUnknownShort'),
                when: item.acked_at
                  ? new Date(item.acked_at).toLocaleString()
                  : '',
              })}
            </p>
          ) : null}
        </div>
      ) : null}
    </li>
  )
}

export function UpgradeHistory() {
  const { t } = useTranslation('updates')
  const [previewing, setPreviewing] = useState<string | null>(null)
  const [showAll, setShowAll] = useState(false)
  const [announce, setAnnounce] = useState('')
  const { data, isPending, isError, error, refetch } = useQuery(
    upgradeHistoryQueryOptions(),
  )

  if (isPending) {
    return (
      <div role="status" aria-label={t('upgradeHistory.loadingLabel')}>
        <ul className="space-y-2" aria-hidden="true">
          {Array.from({ length: 3 }, (_, i) => (
            <li key={i}>
              <Skeleton className="h-9 w-full" />
            </li>
          ))}
        </ul>
      </div>
    )
  }
  if (isError) {
    return (
      <Alert variant="destructive">
        <AlertTitle>
          {t('upgradeHistory.loadError', { message: error.message })}
        </AlertTitle>
        <AlertDescription>
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => {
              void refetch()
            }}
          >
            {t('history.retry')}
          </Button>
        </AlertDescription>
      </Alert>
    )
  }

  // The API returns every row newest first; the cap is applied here.
  const total = data.entries.length
  const capped = total > PREVIEW_COUNT
  const visible = showAll ? data.entries : data.entries.slice(0, PREVIEW_COUNT)
  const latest = data.entries[0]

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <Collapsible>
          <CollapsibleTrigger className="inline-flex min-h-10 w-auto items-center gap-1 text-xs text-muted-foreground hover:text-foreground sm:min-h-6">
            <InfoGlassIcon className="size-4" aria-hidden="true" />
            {t('upgradeHistory.howTitle')}
          </CollapsibleTrigger>
          <CollapsiblePanel>
            <p className="max-w-prose pb-1 text-xs text-muted-foreground">
              {t('upgradeHistory.howBody')}
            </p>
          </CollapsiblePanel>
        </Collapsible>
        {latest ? (
          <span className="text-xs text-muted-foreground">
            {t('upgradeHistory.lastRecorded', {
              when: formatRelative(latest.occurred_at),
            })}
          </span>
        ) : null}
      </div>

      {data.unacknowledged > 0 ? (
        <Alert className="py-1.5">
          <AlertTitle>
            {t('upgradeHistory.pendingBanner', { count: data.unacknowledged })}
          </AlertTitle>
        </Alert>
      ) : null}

      <p role="status" aria-live="polite" className="sr-only">
        {announce}
      </p>

      {total === 0 ? (
        <EmptyState
          compact
          icon={<ClockGlassIcon className="size-5" />}
          title={t('upgradeHistory.empty')}
          description={t('upgradeHistory.emptyHint')}
        />
      ) : (
        <>
          <ol
            aria-label={t('upgradeHistory.list')}
            className={cn(
              'divide-y divide-border',
              showAll && 'max-h-[32rem] overflow-y-auto',
            )}
          >
            {visible.map((item) => (
              <HistoryRow
                key={item.id}
                item={item}
                onRollback={setPreviewing}
                onAcked={(v) => {
                  setAnnounce(t('upgradeHistory.ackAnnounce', { version: v }))
                }}
              />
            ))}
          </ol>
          {capped ? (
            <Button
              type="button"
              size="sm"
              variant="ghost"
              className="min-h-10 sm:min-h-8"
              aria-expanded={showAll}
              onClick={() => {
                setShowAll((v) => !v)
              }}
            >
              {showAll
                ? t('upgradeHistory.showLess', { count: PREVIEW_COUNT })
                : t('upgradeHistory.showAll', { count: total })}
            </Button>
          ) : null}
        </>
      )}

      <AgentVersionTimeline changes={data.agent_changes} />

      <RollbackPlanDialog
        version={previewing}
        onClose={() => {
          setPreviewing(null)
        }}
      />
    </div>
  )
}
