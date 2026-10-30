import { useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { useVirtualizer } from '@tanstack/react-virtual'
import { RefreshCwGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import type { AppKeyStatus } from '../../queries/encryptionKeys'

const ROW_HEIGHT = 56
const VIRTUALIZE_ABOVE = 50

function AppKeyRow({
  entry,
  onRotate,
}: Readonly<{ entry: AppKeyStatus; onRotate: (app: string) => void }>) {
  const { t } = useTranslation('encryptionKeys')
  const when = new Date(
    entry.rotated_at ?? entry.created_at,
  ).toLocaleDateString()
  return (
    <div className="flex h-14 items-center justify-between gap-3 border-b border-border px-3 last:border-b-0">
      <div className="min-w-0">
        <div className="flex items-center gap-2">
          <span className="truncate text-sm font-medium">{entry.app}</span>
          {entry.overdue ? (
            <Badge variant="destructive">{t('apps.overdue')}</Badge>
          ) : null}
        </div>
        <p className="truncate text-xs text-muted-foreground">
          {t('apps.age', { count: entry.age_days })}
          {' · '}
          {t('apps.values', { count: entry.value_count })}
          {' · '}
          {entry.rotated_at
            ? t('apps.rotatedOn', { date: when })
            : t('apps.neverRotated', { date: when })}
        </p>
      </div>
      <Button
        variant="outline"
        size="sm"
        onClick={() => onRotate(entry.app)}
        aria-label={`${t('apps.rotate')}: ${entry.app}`}
      >
        <RefreshCwGlassIcon className="size-5" aria-hidden="true" />
        {t('apps.rotate')}
      </Button>
    </div>
  )
}

function VirtualRows({
  apps,
  onRotate,
}: Readonly<{ apps: AppKeyStatus[]; onRotate: (app: string) => void }>) {
  const parentRef = useRef<HTMLDivElement>(null)
  const virtualizer = useVirtualizer({
    count: apps.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 8,
  })
  return (
    <div ref={parentRef} className="max-h-80 overflow-auto">
      <div className="relative" style={{ height: virtualizer.getTotalSize() }}>
        {virtualizer.getVirtualItems().map((row) => {
          const entry = apps[row.index]
          if (!entry) return null
          return (
            <div
              key={row.key}
              className="absolute top-0 left-0 w-full"
              style={{ transform: `translateY(${row.start}px)` }}
            >
              <AppKeyRow entry={entry} onRotate={onRotate} />
            </div>
          )
        })}
      </div>
    </div>
  )
}

export function AppKeyList({
  apps,
  onRotate,
}: Readonly<{ apps: AppKeyStatus[]; onRotate: (app: string) => void }>) {
  return (
    <div className="rounded-lg border border-border bg-card">
      {apps.length > VIRTUALIZE_ABOVE ? (
        <VirtualRows apps={apps} onRotate={onRotate} />
      ) : (
        apps.map((entry) => (
          <AppKeyRow key={entry.app} entry={entry} onRotate={onRotate} />
        ))
      )}
    </div>
  )
}
