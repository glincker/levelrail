import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { FileSearchGlassIcon } from '@thesvg/ui-react'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { ResourceExplorer } from '@/components/resources/ResourceExplorer'
import { isResourceKind } from '@/lib/resourceLinks'
import type { ConsoleQuery, ResourceKind } from '@/queries/resources'

interface ResourcesSearch {
  q?: string
  kind?: ResourceKind
  tag?: string[]
}

// The filters live in the URL so the palette, saved views and shared links can all open a filtered list.
export const Route = createFileRoute('/resources')({
  validateSearch: (search: Record<string, unknown>): ResourcesSearch => {
    const out: ResourcesSearch = {}
    if (typeof search.q === 'string' && search.q) out.q = search.q
    if (isResourceKind(search.kind)) out.kind = search.kind
    const tags = (Array.isArray(search.tag) ? search.tag : [search.tag]).filter(
      (t): t is string => typeof t === 'string' && t !== '',
    )
    if (tags.length > 0) out.tag = tags
    return out
  },
  component: ResourcesPage,
})

function ResourcesPage() {
  const { t } = useTranslation('resources')
  const search = Route.useSearch()
  const navigate = useNavigate({ from: Route.fullPath })
  const query: ConsoleQuery = {
    q: search.q ?? '',
    kind: search.kind ?? '',
    tags: search.tag ?? [],
  }
  return (
    <PageTemplate
      title={t('title')}
      description={t('description')}
      icon={<FileSearchGlassIcon size="1.2em" />}
      width="wide"
    >
      <ResourceExplorer
        query={query}
        onQueryChange={(q) =>
          void navigate({
            search: {
              ...(q.q ? { q: q.q } : {}),
              ...(q.kind ? { kind: q.kind } : {}),
              ...(q.tags.length > 0 ? { tag: q.tags } : {}),
            },
            replace: true,
          })
        }
      />
    </PageTemplate>
  )
}
