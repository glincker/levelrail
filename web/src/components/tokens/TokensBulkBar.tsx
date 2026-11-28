import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'

interface TokensBulkBarProps {
  count: number
  filterMode: boolean
  // Total matching the filter when every loaded row is selected and more exist.
  offerAllMatching: number | null
  disabledReason: string | null
  onSelectAllMatching: () => void
  onClear: () => void
  onRevoke: () => void
}

// Sticky so it stays reachable while scrolling a long list, including at phone width.
export function TokensBulkBar({
  count,
  filterMode,
  offerAllMatching,
  disabledReason,
  onSelectAllMatching,
  onClear,
  onRevoke,
}: TokensBulkBarProps) {
  const { t } = useTranslation('tokenList')
  if (count === 0) return null
  return (
    <div
      role="region"
      aria-label={t('bar.label')}
      className="sticky bottom-3 z-20 mt-3 flex flex-wrap items-center gap-x-4 gap-y-2 rounded-(--glinr-radius-control) border border-border bg-popover px-4 py-2.5 text-sm text-popover-foreground shadow-(--glinr-elev-2)"
    >
      <span className="font-medium">
        {filterMode
          ? t('bar.allMatching', { count })
          : t('bar.selected', { count })}
      </span>
      {offerAllMatching !== null && !filterMode ? (
        <button
          type="button"
          onClick={onSelectAllMatching}
          className="text-primary underline underline-offset-4 hover:no-underline focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
        >
          {t('bar.selectAllMatching', { count: offerAllMatching })}
        </button>
      ) : null}
      {disabledReason ? (
        <span id="tokens-bulk-reason" className="text-xs text-muted-foreground">
          {disabledReason}
        </span>
      ) : null}
      <span className="ml-auto flex items-center gap-2">
        <Button variant="ghost" size="sm" onClick={onClear}>
          {t('bar.clear')}
        </Button>
        <Button
          variant="destructive"
          size="sm"
          disabled={disabledReason !== null}
          aria-describedby={disabledReason ? 'tokens-bulk-reason' : undefined}
          onClick={onRevoke}
        >
          {t('bar.revoke')}
        </Button>
      </span>
    </div>
  )
}
