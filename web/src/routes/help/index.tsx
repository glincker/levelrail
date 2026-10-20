import { createFileRoute, Link } from '@tanstack/react-router'
import { LifebuoyIcon } from '@phosphor-icons/react/dist/ssr'
import { loadDocsManifest } from '../../lib/docsManifestLoader'

export const Route = createFileRoute('/help/')({
  loader: () => loadDocsManifest(),
  component: HelpIndexPage,
})

function HelpIndexPage() {
  const manifest = Route.useLoaderData()

  return (
    <div className="space-y-6">
      <div className="flex items-start gap-3">
        <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
          <LifebuoyIcon className="size-4" aria-hidden="true" />
        </div>
        <div>
          <h2 className="text-base font-semibold text-foreground">
            Documentation
          </h2>
          <p className="mt-1 text-sm text-muted-foreground">
            Every guide bundled with this control plane, searchable and readable
            without an internet connection. Pick a topic from the sidebar, or
            search above.
          </p>
        </div>
      </div>

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        {manifest.categories.map((category) => (
          <div
            key={category.name}
            className="rounded-lg border border-border p-4"
          >
            <h3 className="mb-2 text-sm font-semibold text-foreground">
              {category.name}
            </h3>
            <ul className="space-y-1">
              {category.docs.map((doc) => (
                <li key={doc.path}>
                  <Link
                    to="/help/$"
                    params={{ _splat: doc.path.slice(1) }}
                    className="text-sm text-primary underline-offset-2 hover:underline"
                  >
                    {doc.title}
                  </Link>
                </li>
              ))}
            </ul>
          </div>
        ))}
      </div>
    </div>
  )
}
