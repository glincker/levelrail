import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type ChangeEvent,
  type MouseEvent,
  type ReactNode,
} from 'react'
import { useTranslation } from 'react-i18next'
import { prefersReducedMotion } from '@/lib/reducedMotion'
import { cn } from '@/lib/utils'

export interface SectionNavItem {
  id: string
  label: string
  /** Decorative glyph shown before the label. */
  icon?: ReactNode
}

// Scrolls we start ourselves ignore the observer until they settle.
const PIN_FALLBACK_MS = 900

function scrollToSection(id: string) {
  const el = document.getElementById(id)
  if (!el) return
  el.scrollIntoView({
    behavior: prefersReducedMotion() ? 'auto' : 'smooth',
    block: 'start',
  })
}

function edgeSection(order: string[]): string | undefined {
  const root = document.documentElement
  if (root.scrollHeight <= window.innerHeight) return undefined
  if (window.scrollY <= 0) return order[0]
  if (window.innerHeight + window.scrollY >= root.scrollHeight - 2)
    return order[order.length - 1]
  return undefined
}

/** Tracks which section id is being read; the page scrolls in the window. */
function useActiveSection(ids: string[]): [string, (id: string) => void] {
  const [active, setActive] = useState(ids[0] ?? '')
  const pinned = useRef(false)
  const pinTimer = useRef<number | undefined>(undefined)
  const key = ids.join('|')
  useEffect(() => {
    if (typeof IntersectionObserver === 'undefined') return
    const visible = new Set<string>()
    const order = key.split('|')
    const observer = new IntersectionObserver(
      (entries) => {
        for (const e of entries) {
          if (e.isIntersecting) visible.add(e.target.id)
          else visible.delete(e.target.id)
        }
        if (pinned.current) return
        const edge = edgeSection(order)
        const first = edge ?? order.find((id) => visible.has(id))
        if (first) setActive(first)
      },
      { rootMargin: '-10% 0px -60% 0px' },
    )
    for (const id of order) {
      const el = document.getElementById(id)
      if (el) observer.observe(el)
    }
    // The band cannot reach the first and last sections at the scroll ends.
    const onScroll = () => {
      if (pinned.current) return
      const edge = edgeSection(order)
      if (edge) setActive(edge)
    }
    window.addEventListener('scroll', onScroll, { passive: true })
    return () => {
      observer.disconnect()
      window.removeEventListener('scroll', onScroll)
    }
  }, [key])
  const choose = useCallback((id: string) => {
    setActive(id)
    pinned.current = true
    window.clearTimeout(pinTimer.current)
    pinTimer.current = window.setTimeout(() => {
      pinned.current = false
    }, PIN_FALLBACK_MS)
  }, [])
  useEffect(() => () => window.clearTimeout(pinTimer.current), [])
  return [active, choose]
}

export interface SectionNavProps {
  items: SectionNavItem[]
  active: string
  onNavigate: (id: string) => void
  /** Accessible name of the nav landmark and the select. */
  label?: string
  className?: string
}

/** SectionNav is a table of contents on xl and a native "Jump to" select below. */
export function SectionNav({
  items,
  active,
  onNavigate,
  label,
  className,
}: SectionNavProps) {
  const { t } = useTranslation('common')
  const jump = label ?? t('kit.jumpTo')
  const onLink = (e: MouseEvent<HTMLAnchorElement>, id: string) => {
    e.preventDefault()
    onNavigate(id)
  }
  const onSelect = (e: ChangeEvent<HTMLSelectElement>) =>
    onNavigate(e.target.value)
  return (
    <div className={className}>
      <div className="xl:hidden">
        <label className="flex items-center gap-2 text-xs text-muted-foreground">
          <span>{jump}</span>
          <select
            value={active}
            onChange={onSelect}
            className="h-8 min-w-0 flex-1 rounded-md border border-input bg-background px-2 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            {items.map((it) => (
              <option key={it.id} value={it.id}>
                {it.label}
              </option>
            ))}
          </select>
        </label>
      </div>
      <nav
        aria-label={jump}
        className="hidden max-h-[calc(100svh-7rem)] overflow-y-auto xl:block"
      >
        <p className="mb-1.5 px-3 text-xs font-medium text-muted-foreground">
          {t('kit.onThisPage')}
        </p>
        <ul className="flex flex-col border-l border-border">
          {items.map((it) => {
            const current = it.id === active
            return (
              <li key={it.id}>
                <a
                  href={`#${it.id}`}
                  aria-current={current ? 'true' : undefined}
                  onClick={(e) => onLink(e, it.id)}
                  className={cn(
                    '-ml-px flex items-center gap-2 border-l-2 border-transparent py-1.5 pr-2 pl-3 text-sm text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring motion-safe:transition-colors hover:text-foreground',
                    current &&
                      'border-state-accent bg-muted/60 font-medium text-foreground',
                  )}
                >
                  {it.icon ? (
                    <span
                      aria-hidden="true"
                      className={cn(
                        'flex shrink-0 items-center',
                        current && 'text-state-accent',
                      )}
                    >
                      {it.icon}
                    </span>
                  ) : null}
                  <span className="truncate">{it.label}</span>
                </a>
              </li>
            )
          })}
        </ul>
      </nav>
    </div>
  )
}

export interface SettingsLayoutProps {
  sections: SectionNavItem[]
  navLabel?: string
  className?: string
  children: ReactNode
}

/** SettingsLayout adds a sticky table of contents beside a page's SettingsSections. */
export function SettingsLayout({
  sections,
  navLabel,
  className,
  children,
}: SettingsLayoutProps) {
  const [active, setActive] = useActiveSection(sections.map((s) => s.id))
  const navigate = useCallback(
    (id: string) => {
      setActive(id)
      scrollToSection(id)
    },
    [setActive],
  )
  return (
    <div
      className={cn(
        'flex flex-col gap-4 xl:grid xl:grid-cols-[11rem_minmax(0,1fr)] xl:items-start xl:gap-8',
        className,
      )}
    >
      <SectionNav
        items={sections}
        active={active}
        onNavigate={navigate}
        label={navLabel}
        className="xl:sticky xl:top-20"
      />
      <div className="min-w-0 space-y-8">{children}</div>
    </div>
  )
}

export interface SettingsSectionProps {
  id: string
  title: string
  description?: string
  icon?: ReactNode
  /** Keep the heading for screen readers only, when the content has its own titles. */
  hideHeading?: boolean
  /** Card columns from lg; 2 by default. */
  columns?: 1 | 2
  children: ReactNode
}

/** SettingsSection is an anchored block with a heading and a card grid. */
export function SettingsSection({
  id,
  title,
  description,
  icon,
  hideHeading,
  columns = 2,
  children,
}: SettingsSectionProps) {
  const headingId = `${id}-heading`
  return (
    <section id={id} aria-labelledby={headingId} className="scroll-mt-20">
      <div className={cn('mb-3', hideHeading && 'sr-only')}>
        <h2
          id={headingId}
          className="flex items-center gap-2 text-base font-medium"
        >
          {icon ? (
            <span className="text-state-accent" aria-hidden="true">
              {icon}
            </span>
          ) : null}
          {title}
        </h2>
        {description ? (
          <p className="text-sm text-muted-foreground">{description}</p>
        ) : null}
      </div>
      <div
        className={cn(
          'grid items-start gap-4',
          columns === 2 && 'lg:grid-cols-2',
        )}
      >
        {children}
      </div>
    </section>
  )
}
