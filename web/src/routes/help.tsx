import {
  createFileRoute,
  Link,
  Outlet,
  useRouterState,
} from '@tanstack/react-router'
import { QuestionIcon } from '@phosphor-icons/react/dist/ssr'
import { HelpSearchBox } from '../components/HelpSearchBox'

// Bundled help layout: every page under it renders from /docs, imported
// at build time (see lib/docsContent.ts and generated/docsManifest.ts),
// so this works fully offline, the same requirement self-hosted install
// already has to meet. Sidebar categories come straight from
// docsManifest, generated from docs/README.md's own index, so it never
// drifts from that file by hand.
//
// The manifest is loaded via the loader's own dynamic import rather than
// a module-level import: this route's shell is reachable from the
// always-loaded app chrome (the header's Help menu), and a static import
// here would pull all 32 pages' headings into the main bundle instead of
// /help's own lazy chunk.
export const Route = createFileRoute('/help')({
  loader: () => import('../generated/docsManifest').then((m) => m.default),
  component: HelpLayout,
})

function HelpLayout() {
  const manifest = Route.useLoaderData()
  const pathname = useRouterState({ select: (s) => s.location.pathname })

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-2">
        <QuestionIcon className="size-5 text-muted-foreground" />
        <h1 className="text-lg font-semibold text-foreground">Help</h1>
      </div>

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-[240px_1fr]">
        <nav className="space-y-4 lg:sticky lg:top-4 lg:self-start">
          <HelpSearchBox manifest={manifest} />
          <div className="space-y-4">
            {manifest.categories.map((category) => (
              <div key={category.name}>
                <p className="mb-1.5 px-2 text-xs font-medium tracking-wide text-muted-foreground uppercase">
                  {category.name}
                </p>
                <ul className="space-y-0.5">
                  {category.docs.map((doc) => {
                    const href = `/help${doc.path}`
                    const active = pathname === href
                    return (
                      <li key={doc.path}>
                        <Link
                          to="/help/$"
                          params={{ _splat: doc.path.slice(1) }}
                          className={`block rounded-md px-2 py-1 text-sm ${
                            active
                              ? 'bg-muted font-medium text-foreground'
                              : 'text-muted-foreground hover:bg-muted hover:text-foreground'
                          }`}
                        >
                          {doc.title}
                        </Link>
                      </li>
                    )
                  })}
                </ul>
              </div>
            ))}
          </div>
        </nav>

        <div className="min-w-0 rounded-lg border border-border bg-card p-4 sm:p-6">
          <Outlet />
        </div>
      </div>
    </div>
  )
}
