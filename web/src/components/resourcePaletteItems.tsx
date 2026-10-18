import * as React from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  ArchiveIcon,
  BookmarkSimpleIcon,
  ListMagnifyingGlassIcon,
  ShareNetworkIcon,
  TagIcon,
} from '@phosphor-icons/react/dist/ssr'
import { fuzzyFilter } from '@/lib/fuzzy'
import { parseView } from '@/lib/resourceLinks'
import {
  resourceKeys,
  useResourceList,
  type ConsoleView,
} from '@/queries/resources'
import { tagsQueryOptions } from '@/queries/tags'
import type { PaletteItem } from './commandPaletteData'

const MAX_TAG_MATCHES = 5
const MIN_QUERY = 2

export interface ResourcePaletteItems {
  base: PaletteItem[]
  search: PaletteItem[]
}

// Resources themselves are listed by the palette per kind; this adds the explorer, saved views and tag search.
export function useResourcePaletteItems(
  open: boolean,
  query: string,
): ResourcePaletteItems {
  const { t } = useTranslation('resources')
  const navigate = useNavigate()
  const tags = useQuery({ ...tagsQueryOptions(), enabled: open })
  const views = useQuery({
    queryKey: resourceKeys.views(),
    queryFn: async ({ signal }): Promise<ConsoleView[]> => {
      const res = await fetch('/api/v1/resources/views', { signal })
      if (!res.ok) return []
      return (await res.json()) as ConsoleView[]
    },
    enabled: open,
    staleTime: 30_000,
  })

  const buckets = useResourceList({ q: '', kind: 'bucket', tags: [] }, open)
  const networks = useResourceList({ q: '', kind: 'network', tags: [] }, open)

  const base = React.useMemo<PaletteItem[]>(() => {
    const items: PaletteItem[] = [
      {
        key: 'action-resource-explorer',
        label: t('palette.open'),
        keywords: t('palette.openKeywords'),
        group: 'Actions',
        icon: <ListMagnifyingGlassIcon />,
        run: () => void navigate({ to: '/resources' }),
      },
    ]
    for (const v of views.data ?? []) {
      const q = parseView(v.query)
      items.push({
        key: `resource-view-${v.id}`,
        label: t('palette.view', { name: v.name }),
        group: 'Saved views',
        icon: <BookmarkSimpleIcon />,
        run: () =>
          void navigate({
            to: '/resources',
            search: {
              ...(q.q ? { q: q.q } : {}),
              ...(q.kind ? { kind: q.kind } : {}),
              ...(q.tags.length > 0 ? { tag: q.tags } : {}),
            },
          }),
      })
    }
    for (const b of buckets.data?.items ?? []) {
      items.push({
        key: `resource-bucket-${b.id ?? b.name}`,
        label: t('palette.bucket', { name: b.name }),
        group: 'Buckets',
        icon: <ArchiveIcon />,
        run: () =>
          void navigate({
            to: '/resources',
            search: { kind: 'bucket', q: b.name },
          }),
      })
    }
    for (const n of networks.data?.items ?? []) {
      items.push({
        key: `resource-network-${n.id ?? n.name}`,
        label: t('palette.network', { name: n.name }),
        group: 'Private networks',
        icon: <ShareNetworkIcon />,
        run: () => void navigate({ to: '/network/private' }),
      })
    }
    return items
  }, [buckets.data, navigate, networks.data, t, views.data])

  const search = React.useMemo<PaletteItem[]>(() => {
    const q = query.trim()
    if (q.length < MIN_QUERY) return []
    return fuzzyFilter(tags.data ?? [], q, (tag) => tag.name)
      .slice(0, MAX_TAG_MATCHES)
      .map((tag) => ({
        key: `resource-tag-${tag.id}`,
        label: t('palette.tagged', { tag: tag.name }),
        group: 'Tags',
        icon: <TagIcon />,
        run: () =>
          void navigate({ to: '/resources', search: { tag: [tag.name] } }),
      }))
  }, [navigate, query, t, tags.data])

  return { base, search }
}
