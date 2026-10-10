import { useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useVirtualizer } from '@tanstack/react-virtual'
import { useTranslation } from 'react-i18next'
import {
  ArrowRightIcon,
  ArrowCounterClockwiseIcon,
  ArrowSquareOutIcon,
  CaretDownIcon,
  CaretRightIcon,
  CheckIcon,
} from '@phosphor-icons/react/dist/ssr'
import { StatusPill, type Tone } from '@/components/kit'
import { formatRelative } from '@/components/kit/formatRelative'
import { Alert, AlertDescription, AlertTitle } from '../ui/alert'
import { Badge } from '../ui/badge'
import { Button } from '../ui/button'
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

const VIRTUALIZE_ABOVE = 50
const ESTIMATED_ROW_PX = 150

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
    <span className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs text-foreground">
      {value}
    </span>
  )
}

function HistoryRow({
  item,
  onRollback,
}: {
  item: UpgradeHistoryItem
  onRollback: (version: string) => void
}) {
  const { t } = useTranslation('updates')
  const ack = useAckUpgrade()
  const [open, setOpen] = useState(false)
  const hasNotes = item.notes_state === 'fetched' && item.notes !== ''
  const absolute = new Date(item.occurred_at).toLocaleString()

  return (
    <div
      role="listitem"
      className="space-y-2 border-l-2 border-border py-3 pl-4"
    >
      {!item.acknowledged ? (
        <div
          role="status"
          className="flex flex-wrap items-center justify-between gap-2 rounded-md border border-amber-500/60 px-3 py-2"
        >
          <span className="text-sm text-foreground">
            {t('upgradeHistory.unacknowledged')}
          </span>
          <Button
            type="button"
            size="sm"
            disabled={ack.isPending}
            onClick={() => {
              ack.mutate(item.id, {
                onSuccess: () => {
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
            <CheckIcon className="size-4" />
            {t('upgradeHistory.acknowledge')}
          </Button>
        </div>
      ) : null}

      <div className="flex flex-wrap items-center gap-2">
        <StatusPill
          tone={KIND_TONE[item.kind]}
          size="sm"
          label={t(`upgradeHistory.kind.${item.kind}`)}
        />
        {item.from_version !== '' ? (
          <>
            <VersionChip value={item.from_version} />
            <ArrowRightIcon className="size-3.5 text-muted-foreground" />
          </>
        ) : null}
        <VersionChip value={item.to_version} />
        {item.channel === 'stable' || item.channel === 'beta' ? (
          <Badge variant="outline">
            {t(`history.channel.${item.channel}`)}
          </Badge>
        ) : null}
        <time
          dateTime={item.occurred_at}
          title={absolute}
          className="text-xs text-muted-foreground"
        >
          {formatRelative(item.occurred_at)}
        </time>
      </div>

      <p className="text-xs text-muted-foreground">
        {item.initiator === 'unknown'
          ? t('upgradeHistory.initiatorUnknown')
          : t('upgradeHistory.initiator', {
              who: item.initiator,
              method: item.method || t('upgradeHistory.methodUnknown'),
            })}
      </p>

      <ul className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
        <li>
          {item.schema_before !== null && item.schema_after !== null
            ? t('upgradeHistory.schemaDelta', {
                before: item.schema_before,
                after: item.schema_after,
              })
            : t('upgradeHistory.schemaUnknown')}
        </li>
        <li>
          {item.backup_name !== ''
            ? t('upgradeHistory.backup', { name: item.backup_name })
            : t('upgradeHistory.noBackup')}
        </li>
        <li>{t(`upgradeHistory.health.${item.health}`, item.health)}</li>
      </ul>

      {item.schema_moved ? (
        <p className="text-xs text-amber-700 dark:text-amber-400">
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
            <ArrowSquareOutIcon className="size-3.5" />
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
            <ArrowSquareOutIcon className="size-3.5" />
          </a>
        ) : null}
        {item.rollback_available ? (
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => {
              onRollback(item.from_version)
            }}
          >
            <ArrowCounterClockwiseIcon className="size-4" />
            {t('upgradeHistory.rollbackTo', { version: item.from_version })}
          </Button>
        ) : null}
        {hasNotes ? (
          <button
            type="button"
            aria-expanded={open}
            className="inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
            onClick={() => {
              setOpen((v) => !v)
            }}
          >
            {open ? (
              <CaretDownIcon className="size-3.5" />
            ) : (
              <CaretRightIcon className="size-3.5" />
            )}
            {t('upgradeHistory.notes')}
          </button>
        ) : (
          <span className="text-xs text-muted-foreground">
            {item.notes_state === 'pending'
              ? t('upgradeHistory.notesPending')
              : t('upgradeHistory.notesUnavailable')}
          </span>
        )}
      </div>

      {open && hasNotes ? <ReleaseNotes markdown={item.notes} /> : null}

      {item.acknowledged ? (
        <p className="text-xs text-muted-foreground">
          {t('upgradeHistory.ackedBy', {
            who: item.acked_by || t('upgradeHistory.initiatorUnknownShort'),
            when: item.acked_at ? new Date(item.acked_at).toLocaleString() : '',
          })}
        </p>
      ) : null}
    </div>
  )
}

function VirtualRows({
  items,
  onRollback,
}: {
  items: UpgradeHistoryItem[]
  onRollback: (version: string) => void
}) {
  const parentRef = useRef<HTMLDivElement>(null)
  const virtualizer = useVirtualizer({
    count: items.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => ESTIMATED_ROW_PX,
    overscan: 6,
  })
  return (
    <div ref={parentRef} className="max-h-[40rem] overflow-y-auto">
      <div
        role="list"
        className="relative"
        style={{ height: `${virtualizer.getTotalSize()}px` }}
      >
        {virtualizer.getVirtualItems().map((v) => {
          const item = items[v.index]
          if (!item) return null
          return (
            <div
              key={item.id}
              data-index={v.index}
              ref={virtualizer.measureElement}
              className="absolute left-0 top-0 w-full"
              style={{ transform: `translateY(${v.start}px)` }}
            >
              <HistoryRow item={item} onRollback={onRollback} />
            </div>
          )
        })}
      </div>
    </div>
  )
}

export function UpgradeHistory() {
  const { t } = useTranslation('updates')
  const [previewing, setPreviewing] = useState<string | null>(null)
  const { data, isPending, isError, error, refetch } = useQuery(
    upgradeHistoryQueryOptions(),
  )

  if (isPending) {
    return (
      <p className="text-sm text-muted-foreground">
        {t('upgradeHistory.loading')}
      </p>
    )
  }
  if (isError) {
    return (
      <div className="space-y-2">
        <p role="alert" className="text-sm text-destructive">
          {t('upgradeHistory.loadError', { message: error.message })}
        </p>
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
      </div>
    )
  }

  return (
    <div className="space-y-4">
      {data.unacknowledged > 0 ? (
        <Alert>
          <AlertTitle>
            {t('upgradeHistory.pendingTitle', { count: data.unacknowledged })}
          </AlertTitle>
          <AlertDescription>{t('upgradeHistory.pendingBody')}</AlertDescription>
        </Alert>
      ) : null}

      {data.entries.length === 0 ? (
        <div className="rounded-md border border-dashed px-4 py-6 text-center">
          <p className="text-sm font-medium text-foreground">
            {t('upgradeHistory.empty')}
          </p>
          <p className="text-xs text-muted-foreground">
            {t('upgradeHistory.emptyHint')}
          </p>
        </div>
      ) : data.entries.length > VIRTUALIZE_ABOVE ? (
        <VirtualRows items={data.entries} onRollback={setPreviewing} />
      ) : (
        <div role="list">
          {data.entries.map((item) => (
            <HistoryRow key={item.id} item={item} onRollback={setPreviewing} />
          ))}
        </div>
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
