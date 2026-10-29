import { useState } from 'react'
import {
  createFileRoute,
  Link,
  Outlet,
  useRouterState,
} from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ChevronDownGlassIcon,
  ChevronUpGlassIcon,
  CircleQuestionMarkGlassIcon,
  ListGlassIcon,
} from '@thesvg/ui-react'
import { HelpSearchBox } from '../components/HelpSearchBox'
import { DocsOnThisPage } from '../components/DocsOnThisPage'
import { loadDocsManifest } from '../lib/docsManifestLoader'
import { getTroubleshootingPages } from '../lib/docsTroubleshootingFilter'
import { usePinnedPages } from '@/hooks/usePinnedPages'
import { useRecentlyViewedPages } from '@/hooks/useRecentlyViewedPages'
import { PageHeader } from '@/components/shell/PageHeader'
import { cn } from '@/lib/utils'

// Bundled help layout: every page under it renders from /docs, imported
// at build time (see lib/docsContent.ts and vite-plugins/docsManifest.mjs),
// so this works fully offline, the same requirement self-hosted install
// already has to meet. Sidebar categories come straight from the
// manifest, generated from docs/README.md's own index, so it never
// drifts from that file by hand.
//
// The manifest is loaded via the loader's own dynamic import rather than
// a module-level import: this route's shell is reachable from the
// always-loaded app chrome (the header's Help menu), and a static import
// here would pull all 32 pages' headings into the main bundle instead of
// /help's own lazy chunk.
export const Route = createFileRoute('/help')({
  loader: () => loadDocsManifest(),
  component: HelpLayout,
})

function HelpLayout() {
  const { t: tHelp } = useTranslation('help')
  const manifest = Route.useLoaderData()
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const currentRoutePath = pathname.replace(/^\/help/, '') || '/'
  const currentPage = manifest.pages[currentRoutePath]
  const { pinned } = usePinnedPages()
  const recentlyViewed = useRecentlyViewedPages(
    currentRoutePath,
    currentPage?.title || '',
  )
  const [showTroubleshootingOnly, setShowTroubleshootingOnly] = useState(false)
  const troubleshootingPages = getTroubleshootingPages(manifest)
  const [expandedSections, setExpandedSections] = useState<Set<string>>(
    new Set(['pinned', 'recent']),
  )

  const toggleSection = (section: string) => {
    const newExpanded = new Set(expandedSections)
    if (newExpanded.has(section)) {
      newExpanded.delete(section)
    } else {
      newExpanded.add(section)
    }
    setExpandedSections(newExpanded)
  }

  const renderDocLink = (
    doc: { path: string; title: string },
    active: boolean,
  ) => {
    return (
      <li key={doc.path}>
        <Link
          to="/help/$"
          params={{ _splat: doc.path.slice(1) }}
          className={cn(
            'block rounded-md px-2 py-1 text-sm',
            active
              ? 'bg-muted font-medium text-foreground'
              : 'text-muted-foreground hover:bg-muted hover:text-foreground',
          )}
        >
          {doc.title}
        </Link>
        {active && currentPage ? (
          <DocsOnThisPage
            key={pathname}
            headings={currentPage.headings}
            className="mt-1 mb-1 ml-2 xl:hidden"
          />
        ) : null}
      </li>
    )
  }

  const renderSection = (
    title: string,
    sectionKey: string,
    docs: Array<{ path: string; title: string }>,
  ) => {
    if (docs.length === 0) return null

    const isExpanded = expandedSections.has(sectionKey)

    return (
      <div key={sectionKey}>
        <button
          type="button"
          onClick={() => toggleSection(sectionKey)}
          className="mb-1.5 w-full flex items-center gap-1.5 px-2 py-1.5 text-xs font-medium tracking-wide text-muted-foreground uppercase hover:text-foreground transition-colors"
          aria-expanded={isExpanded}
        >
          {isExpanded ? (
            <ChevronDownGlassIcon className="size-3.5" />
          ) : (
            <ChevronUpGlassIcon className="size-3.5" />
          )}
          {title}
        </button>
        {isExpanded && (
          <ul className="space-y-0.5 mb-4">
            {docs.map((doc) => {
              const href = `/help${doc.path}`
              const active = pathname === href
              return renderDocLink(doc, active)
            })}
          </ul>
        )}
      </div>
    )
  }

  const filteredCategories = showTroubleshootingOnly
    ? manifest.categories.map((cat) => ({
        ...cat,
        docs: cat.docs.filter((doc) => troubleshootingPages.has(doc.path)),
      }))
    : manifest.categories

  const isTroubleshootingFilterable =
    Array.from(troubleshootingPages).length > 0

  return (
    <div className="space-y-6">
      <PageHeader
        title={
          <span className="flex items-center gap-2">
            <CircleQuestionMarkGlassIcon className="size-5.5 text-muted-foreground" />
            Help
          </span>
        }
      />

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-[240px_minmax(0,1fr)] xl:grid-cols-[240px_minmax(0,1fr)_200px]">
        <nav className="flex flex-col gap-4 lg:sticky lg:top-4 lg:max-h-[calc(100vh-2rem)] lg:self-start lg:overflow-y-auto">
          <HelpSearchBox manifest={manifest} />

          {isTroubleshootingFilterable && (
            <button
              type="button"
              onClick={() =>
                setShowTroubleshootingOnly(!showTroubleshootingOnly)
              }
              className={cn(
                'rounded-md px-2 py-1 text-xs font-medium transition-colors',
                showTroubleshootingOnly
                  ? 'bg-primary text-primary-foreground'
                  : 'bg-muted text-muted-foreground hover:bg-muted/80 hover:text-foreground',
              )}
            >
              {tHelp('filter.troubleshootingOnly')}
            </button>
          )}

          <div className="space-y-4">
            {pinned.length > 0 &&
              renderSection(
                tHelp('nav.pinned'),
                'pinned',
                pinned.map((p) => ({ path: p.path, title: p.title })),
              )}

            {recentlyViewed.length > 0 &&
              renderSection(
                tHelp('nav.recentlyViewed'),
                'recent',
                recentlyViewed.map((p) => ({ path: p.path, title: p.title })),
              )}

            {filteredCategories.map((category) => (
              <div key={category.name}>
                <p className="mb-1.5 px-2 text-xs font-medium tracking-wide text-muted-foreground uppercase">
                  {category.name}
                </p>
                <ul className="space-y-0.5">
                  {category.docs.map((doc) => {
                    const href = `/help${doc.path}`
                    const active = pathname === href
                    return renderDocLink(doc, active)
                  })}
                </ul>
              </div>
            ))}
          </div>
        </nav>

        <div className="min-w-0 rounded-lg border border-border bg-card p-4 sm:p-6">
          <Outlet />
        </div>
        {currentPage &&
        currentRoutePath !== '/' &&
        currentPage.headings.length > 0 ? (
          <aside
            aria-label={tHelp('toc.title')}
            className="sticky top-4 hidden max-h-[calc(100vh-2rem)] self-start overflow-y-auto xl:block"
          >
            <p className="mb-2 flex items-center gap-1.5 text-xs font-medium tracking-wide text-muted-foreground uppercase">
              <ListGlassIcon className="size-4" aria-hidden="true" />
              {tHelp('toc.title')}
            </p>
            <DocsOnThisPage
              key={`rail-${pathname}`}
              headings={currentPage.headings}
              className="border-l border-border pl-3"
            />
          </aside>
        ) : null}
      </div>
    </div>
  )
}
