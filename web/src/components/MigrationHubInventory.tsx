import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useVirtualizer } from '@tanstack/react-virtual'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import {
  useSelectHubItems,
  useSetHubStep,
  type HubItem,
  type HubSession,
} from '../queries/migrationHub'
import { formatBytes } from './migrationHubFormat'

const VIRTUAL_THRESHOLD = 50
const ROW_HEIGHT = 52
const GRID =
  'grid grid-cols-[2rem_1.4fr_6rem_4rem_1fr_1.2fr] items-center gap-3'

function Row({
  item,
  locked,
  onToggle,
  onRename,
}: {
  item: HubItem
  locked: boolean
  onToggle: (v: boolean) => void
  onRename: (v: string) => void
}) {
  const { t } = useTranslation('migration')
  const [name, setName] = useState(item.target_name)
  return (
    <div className={`${GRID} border-b px-3 py-2 text-sm`}>
      <Checkbox
        aria-label={t('hub.inventory.selectOne', { name: item.source_db })}
        checked={item.selected}
        disabled={locked}
        onCheckedChange={(v) => onToggle(v === true)}
      />
      <span className="truncate font-medium">{item.source_db}</span>
      <span className="tabular-nums">{formatBytes(item.size_bytes)}</span>
      <span className="tabular-nums">{item.tables}</span>
      <span className="flex flex-wrap gap-1">
        {(item.extensions ?? []).map((e) => (
          <Badge key={e} variant="muted">
            {e}
          </Badge>
        ))}
      </span>
      <Input
        aria-label={t('hub.inventory.targetLabel', { name: item.source_db })}
        value={name}
        disabled={locked}
        onChange={(e) => setName(e.target.value)}
        onBlur={() => name !== item.target_name && onRename(name)}
        className="h-8"
      />
    </div>
  )
}

function Header({
  all,
  some,
  locked,
  onAll,
}: {
  all: boolean
  some: boolean
  locked: boolean
  onAll: (v: boolean) => void
}) {
  const { t } = useTranslation('migration')
  return (
    <div
      className={`${GRID} border-b bg-muted/40 px-3 py-2 text-xs font-medium text-muted-foreground`}
    >
      <Checkbox
        aria-label={t('hub.inventory.selectAll')}
        checked={all}
        indeterminate={some && !all}
        disabled={locked}
        onCheckedChange={(v) => onAll(v === true)}
      />
      <span>{t('hub.inventory.columns.source')}</span>
      <span>{t('hub.inventory.columns.size')}</span>
      <span>{t('hub.inventory.columns.tables')}</span>
      <span>{t('hub.inventory.columns.extensions')}</span>
      <span>{t('hub.inventory.columns.target')}</span>
    </div>
  )
}

export function MigrationHubInventory({
  session,
  onNext,
}: {
  session: HubSession
  onNext: () => void
}) {
  const { t } = useTranslation('migration')
  const select = useSelectHubItems()
  const setStep = useSetHubStep()
  const scroller = useRef<HTMLDivElement>(null)
  const items = session.items
  const locked = session.running || select.isPending
  const virtual = items.length > VIRTUAL_THRESHOLD
  const virtualizer = useVirtualizer({
    count: virtual ? items.length : 0,
    getScrollElement: () => scroller.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 8,
  })

  function update(
    sourceDb: string,
    patch: { selected?: boolean; target_name?: string },
  ) {
    select.mutate({
      id: session.id,
      items: [{ source_db: sourceDb, ...patch }],
    })
  }
  function setAll(v: boolean) {
    select.mutate({
      id: session.id,
      items: items.map((i) => ({ source_db: i.source_db, selected: v })),
    })
  }
  const selectedCount = items.filter((i) => i.selected).length

  if (items.length === 0) {
    return (
      <Alert>
        <AlertDescription>{t('hub.inventory.empty')}</AlertDescription>
      </Alert>
    )
  }

  const rowFor = (item: HubItem) => (
    <Row
      key={`${item.source_db}:${item.target_name}`}
      item={item}
      locked={locked}
      onToggle={(v) => update(item.source_db, { selected: v })}
      onRename={(v) => update(item.source_db, { target_name: v })}
    />
  )

  return (
    <div className="space-y-4">
      <div className="space-y-1">
        <h3 className="text-base font-semibold">{t('hub.inventory.title')}</h3>
        <p className="text-sm text-muted-foreground">
          {t('hub.inventory.summary', {
            count: items.length,
            size: formatBytes(session.summary.total_bytes),
            version: session.server_version,
            free: formatBytes(session.free_bytes),
          })}
        </p>
      </div>
      <div className="rounded-md border">
        <Header
          all={selectedCount === items.length}
          some={selectedCount > 0}
          locked={locked}
          onAll={setAll}
        />
        {virtual ? (
          <div ref={scroller} className="max-h-96 overflow-auto">
            <div
              className="relative"
              style={{ height: virtualizer.getTotalSize() }}
            >
              {virtualizer.getVirtualItems().map((v) => (
                <div
                  key={v.key}
                  className="absolute inset-x-0"
                  style={{ top: v.start, height: v.size }}
                >
                  {items[v.index] ? rowFor(items[v.index] as HubItem) : null}
                </div>
              ))}
            </div>
          </div>
        ) : (
          items.map(rowFor)
        )}
      </div>
      {select.isError ? (
        <Alert variant="destructive">
          <AlertDescription>{select.error.message}</AlertDescription>
        </Alert>
      ) : null}
      {selectedCount === 0 ? (
        <p className="text-sm text-muted-foreground">
          {t('hub.inventory.noneSelected')}
        </p>
      ) : null}
      <Button
        disabled={selectedCount === 0 || locked || setStep.isPending}
        onClick={() =>
          setStep.mutate(
            { id: session.id, step: 'preflight' },
            { onSuccess: onNext },
          )
        }
      >
        {t('hub.inventory.next')}
      </Button>
    </div>
  )
}
