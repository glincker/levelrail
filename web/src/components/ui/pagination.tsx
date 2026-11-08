import { useTranslation } from 'react-i18next'
import { ChevronRightGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'

// Prev/next pager with a page count; the caller owns the page state.
export function Pagination({
  page,
  pageCount,
  onPageChange,
}: {
  page: number
  pageCount: number
  onPageChange: (page: number) => void
}) {
  const { t } = useTranslation('uiShared')
  return (
    <nav
      aria-label={t('pagination.label')}
      className="flex items-center justify-end gap-2"
    >
      <span className="text-sm text-muted-foreground tabular-nums">
        {t('pagination.status', { page, count: pageCount })}
      </span>
      <Button
        variant="outline"
        size="icon"
        aria-label={t('pagination.previous')}
        disabled={page <= 1}
        onClick={() => onPageChange(page - 1)}
      >
        <ChevronRightGlassIcon aria-hidden="true" className="rotate-180" />
      </Button>
      <Button
        variant="outline"
        size="icon"
        aria-label={t('pagination.next')}
        disabled={page >= pageCount}
        onClick={() => onPageChange(page + 1)}
      >
        <ChevronRightGlassIcon aria-hidden="true" />
      </Button>
    </nav>
  )
}
