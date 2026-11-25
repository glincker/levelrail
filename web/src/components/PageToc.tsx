import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { createScrollSpy, type TocItem } from './pageTocSpy'

export type { TocItem } from './pageTocSpy'

function prefersReducedMotion(): boolean {
  return (
    typeof window !== 'undefined' &&
    typeof window.matchMedia === 'function' &&
    window.matchMedia('(prefers-reduced-motion: reduce)').matches
  )
}

function hashId(): string | null {
  const raw = window.location.hash.replace(/^#/, '')
  return raw ? decodeURIComponent(raw) : null
}

/** Small right rail of dots, one per section; labels open on hover or focus. Hidden under xl. */
export function PageToc({ items }: Readonly<{ items: readonly TocItem[] }>) {
  const { t } = useTranslation('dbOverview')
  const [active, setActive] = useState<string | null>(null)
  const ids = useMemo(() => items.map((i) => i.id), [items])
  const idKey = ids.join('|')

  const scrollTo = useCallback((id: string) => {
    const el = document.getElementById(id)
    if (!el) return
    el.scrollIntoView({
      behavior: prefersReducedMotion() ? 'auto' : 'smooth',
      block: 'start',
    })
  }, [])

  useEffect(() => {
    const present = ids.filter((id) => document.getElementById(id))
    const stop = createScrollSpy(present, setActive)
    // A deep link lands after lazy cards mount, so re-scroll once content settles.
    const target = hashId()
    let raf = 0
    if (target && present.includes(target)) {
      raf = window.requestAnimationFrame(() => {
        setActive(target)
        document.getElementById(target)?.scrollIntoView({ block: 'start' })
      })
    }
    return () => {
      stop()
      window.cancelAnimationFrame(raf)
    }
    // idKey stands in for ids so a new array with the same sections does not re-observe.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [idKey])

  return (
    <nav
      aria-label={t('toc.label')}
      className="group/toc fixed top-1/2 right-3 z-30 hidden -translate-y-1/2 xl:block"
    >
      <ul className="flex flex-col rounded-lg border border-transparent p-1 transition-colors duration-150 group-hover/toc:border-border group-hover/toc:bg-card/95 group-hover/toc:shadow-sm group-focus-within/toc:border-border group-focus-within/toc:bg-card/95 group-focus-within/toc:shadow-sm motion-reduce:transition-none">
        {items.map((item) => {
          const on = item.id === active
          return (
            <li key={item.id}>
              <a
                href={`#${item.id}`}
                aria-current={on ? 'location' : undefined}
                onClick={(e) => {
                  e.preventDefault()
                  scrollTo(item.id)
                  window.history.replaceState(null, '', `#${item.id}`)
                  setActive(item.id)
                }}
                className="flex h-6 items-center justify-end gap-2 rounded px-1.5 text-xs text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
              >
                <span
                  className={cn(
                    'max-w-0 overflow-hidden whitespace-nowrap opacity-0 transition-[max-width,opacity] duration-150 group-hover/toc:max-w-40 group-hover/toc:opacity-100 group-focus-within/toc:max-w-40 group-focus-within/toc:opacity-100 motion-reduce:transition-none',
                    on && 'font-medium text-foreground',
                  )}
                >
                  {item.label}
                </span>
                <span
                  aria-hidden="true"
                  className={cn(
                    'block rounded-full transition-all duration-150 motion-reduce:transition-none',
                    on
                      ? 'h-2 w-2 bg-primary'
                      : 'h-1.5 w-1.5 bg-muted-foreground/40',
                  )}
                />
              </a>
            </li>
          )
        })}
      </ul>
    </nav>
  )
}
