import { createFileRoute, Link } from '@tanstack/react-router'
import { WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { PageSpinner } from '@/components/ui/page-spinner'
import { DocsRenderer } from '../../components/DocsRenderer'
import { loadDocContent } from '../../lib/docsContent'
import { loadDocsManifest } from '../../lib/docsManifestLoader'
import { useBrand } from '../../hooks/useBrand'

// Splat route: catches every doc path at any depth ("/help/troubleshooting",
// "/help/design/git-provider-integrations") in one file, since every
// bundled doc is reachable the same way regardless of nesting.
export const Route = createFileRoute('/help/$')({
  loader: async ({ params }) => {
    const manifest = await loadDocsManifest()
    const routePath = `/${params._splat ?? ''}`
    const page = manifest.pages[routePath]
    if (!page) return { routePath, manifest, page: null, content: null }
    const content = await loadDocContent(page.file)
    return { routePath, manifest, page, content }
  },
  component: HelpDocPage,
  pendingComponent: () => <PageSpinner />,
})

function HelpDocPage() {
  const { routePath, manifest, page, content } = Route.useLoaderData()
  const brand = useBrand()

  if (!page || content === null) {
    return (
      <div className="flex items-start gap-3 rounded-md border border-border p-4 text-sm">
        <WarningIcon className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
        <div>
          <p className="font-medium text-foreground">Page not found</p>
          <p className="mt-1 text-muted-foreground">
            {routePath} isn&rsquo;t a bundled doc.{' '}
            <Link to="/help" className="underline underline-offset-2">
              Back to Help
            </Link>
          </p>
        </div>
      </div>
    )
  }

  return (
    <DocsRenderer
      markdown={content}
      currentFile={page.file}
      manifest={manifest}
      docsBaseUrl={brand.DocsURL}
    />
  )
}
