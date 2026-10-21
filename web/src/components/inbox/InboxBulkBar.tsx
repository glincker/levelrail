import { useTranslation } from 'react-i18next'
import {
  ArchiveIcon,
  CheckIcon,
  TrashIcon,
  XIcon,
} from '@phosphor-icons/react/dist/ssr'
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
        <ArchiveIcon />
        {t(p.archivedView ? 'page.restore' : 'page.archive')}
      </Button>
      <Button
        type="button"
        size="sm"
        variant="outline"
        disabled={p.busy}
        onClick={p.onMarkRead}
      >
        <CheckIcon />
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
        <TrashIcon />
        {t('bulk.delete')}
      </Button>
      <Button
        type="button"
        size="sm"
        variant="ghost"
        className="ml-auto"
        onClick={p.onClear}
      >
        <XIcon />
        {t('bulk.clear')}
      </Button>
    </div>
  )
}
