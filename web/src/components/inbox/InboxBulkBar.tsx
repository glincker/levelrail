import { useTranslation } from 'react-i18next'
import {
  ArchiveGlassIcon,
  CheckGlassIcon,
  TrashGlassIcon,
  XGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'

export interface InboxBulkBarProps {
  count: number
  canDelete: boolean
  archivedView: boolean
  busy: boolean
  onArchive: () => void
  onMarkRead: () => void
  onDelete: () => void
  onClear: () => void
}

export default function InboxBulkBar(p: InboxBulkBarProps) {
  const { t } = useTranslation('inbox')
  return (
    <div
      role="toolbar"
      aria-label={t('bulk.label')}
      className="flex flex-wrap items-center gap-2 rounded-lg border bg-muted px-3 py-2"
    >
      <span className="text-sm font-medium">
        {t('bulk.selected', { count: p.count })}
      </span>
      <Button
        type="button"
        size="sm"
        variant="outline"
        disabled={p.busy}
        onClick={p.onArchive}
      >
        <ArchiveGlassIcon size="1.4em" />
        {t(p.archivedView ? 'page.restore' : 'page.archive')}
      </Button>
      <Button
        type="button"
        size="sm"
        variant="outline"
        disabled={p.busy}
        onClick={p.onMarkRead}
      >
        <CheckGlassIcon size="1.4em" />
        {t('markRead')}
      </Button>
      <Button
        type="button"
        size="sm"
        variant="destructive"
        disabled={p.busy || !p.canDelete}
        title={p.canDelete ? undefined : t('bulk.deleteBlocked')}
        onClick={p.onDelete}
      >
        <TrashGlassIcon size="1.4em" />
        {t('bulk.delete')}
      </Button>
      <Button
        type="button"
        size="sm"
        variant="ghost"
        className="ml-auto"
        onClick={p.onClear}
      >
        <XGlassIcon size="1.4em" />
        {t('bulk.clear')}
      </Button>
    </div>
  )
}
