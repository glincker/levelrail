import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from './button'

// Fetches the next page when scrolled into view; the button is the fallback
// for keyboards and browsers without IntersectionObserver.
export function LoadMore({
  loaded,
  total,
  loading,
  onLoadMore,
}: {
  loaded: number
  total: number
  loading: boolean
  onLoadMore: () => void
}) {
  const { t } = useTranslation('common')
  const ref = useRef<HTMLDivElement>(null)
  const more = loaded < total

  useEffect(() => {
    const el = ref.current
    if (!more || loading || !el || typeof IntersectionObserver === 'undefined')
      return undefined
    const io = new IntersectionObserver((entries) => {
      if (entries.some((e) => e.isIntersecting)) onLoadMore()
    })
    io.observe(el)
    return () => {
      io.disconnect()
    }
  }, [more, loading, onLoadMore])

  if (!more) return null
  return (
    <div ref={ref} className="flex items-center justify-center gap-3 py-3">
      <span className="text-xs text-muted-foreground">
        {t('loadMore.showing', { loaded, total })}
      </span>
      <Button
        size="sm"
        variant="outline"
        disabled={loading}
        onClick={onLoadMore}
      >
        {t('loadMore.button')}
      </Button>
    </div>
  )
}
