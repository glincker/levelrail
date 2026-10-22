import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { formatRelativeTime } from '../lib/relativeTime'

interface DraftControls {
  hasDraft: boolean
  savedAt: number | null
  restore: () => unknown
  discard: () => void
  dismiss: () => void
}

export function DraftBanner({
  draft,
  onRestore,
}: Readonly<{
  draft: DraftControls
  onRestore?: () => void
}>) {
  const { t } = useTranslation('common')
  if (!draft.hasDraft || draft.savedAt === null) return null
  return (
    <div
      role="status"
      className="flex flex-wrap items-center gap-x-3 gap-y-2 rounded-md border border-border bg-muted/40 px-3 py-2 text-sm"
    >
      <p className="min-w-0 flex-1 text-muted-foreground">
        {t('draft.message', {
          time: formatRelativeTime(new Date(draft.savedAt).toISOString()),
        })}
      </p>
      <div className="flex flex-wrap items-center gap-2">
        <Button
          type="button"
          size="sm"
          onClick={() => {
            draft.restore()
            onRestore?.()
          }}
        >
          {t('draft.continue')}
        </Button>
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={draft.discard}
        >
          {t('draft.startOver')}
        </Button>
        <Button type="button" size="sm" variant="ghost" onClick={draft.dismiss}>
          {t('draft.dismiss')}
        </Button>
      </div>
    </div>
  )
}
