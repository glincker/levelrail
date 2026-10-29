import * as React from 'react'
import { useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  BookOpenGlassIcon,
  BotGlassIcon,
  PackageGlassIcon,
} from '@thesvg/ui-react'
import type { PaletteMode } from '@/lib/paletteMode'
import { searchDocsGrouped } from '@/lib/docsSearch'
import { loadDocsManifest } from '@/lib/docsManifestLoader'
import type { PaletteItem } from '../components/commandPaletteData'
import { dockerHubKeys, fetchDockerHubSearch } from '../queries/dockerhub'
import { useDebouncedValue } from './useDebouncedValue'

const MIN_TERM = 2
const MAX_DOCS = 8
const MAX_IMAGES = 8
const DEBOUNCE_MS = 250

export interface ModeItems {
  header: string
  items: PaletteItem[]
}

// Results for the prefix modes. Docker Hub is only asked when the user typed
// "docker ", "deploy ", "new " or "+", so ordinary searches never leave the server.
export function usePaletteModeItems(
  mode: PaletteMode | null,
): ModeItems | null {
  const { t } = useTranslation('palette')
  const navigate = useNavigate()
  const term = mode?.term ?? ''
  const debounced = useDebouncedValue(term, DEBOUNCE_MS)
  const ready = debounced.length >= MIN_TERM

  const docs = useQuery({
    queryKey: ['palette', 'docs-manifest'],
    queryFn: loadDocsManifest,
    enabled: mode?.kind === 'docs',
    staleTime: Infinity,
  })
  const images = useQuery({
    queryKey: dockerHubKeys.search(debounced),
    queryFn: () => fetchDockerHubSearch(debounced),
    enabled: mode?.kind === 'docker' && ready,
    staleTime: 60_000,
  })

  return React.useMemo<ModeItems | null>(() => {
    if (!mode) return null
    const hint = (key: string, label: string, icon: React.ReactNode) => [
      {
        key,
        label,
        group: mode.kind,
        icon,
        keepOpen: true,
        run: () => undefined,
      },
    ]
    if (mode.kind === 'docs') {
      const icon = <BookOpenGlassIcon size="1.4em" />
      if (!ready) {
        return {
          header: t('modes.docs.header'),
          items: hint('mode-docs-hint', t('modes.docs.hint'), icon),
        }
      }
      if (!docs.data) {
        return {
          header: t('modes.docs.header'),
          items: hint('mode-docs-loading', t('loading'), icon),
        }
      }
      const manifest = docs.data
      const items: PaletteItem[] = searchDocsGrouped(manifest, debounced, {
        pages: MAX_DOCS,
        headings: 2,
      }).flatMap((page) => {
        const open = (hash?: string) => () =>
          void navigate({
            to: '/help/$',
            params: { _splat: page.path.replace(/^\//, '') },
            hash,
          })
        const excerpt = (manifest.pages[page.path]?.body ?? '').slice(0, 180)
        const preview = {
          kind: 'doc' as const,
          description: excerpt,
          path: `/help${page.path}`,
          primaryLabel: t('modes.docs.open'),
        }
        const rows: PaletteItem[] = [
          {
            key: `mode-docs-${page.path}`,
            label: page.title,
            subtitle: page.path,
            stacked: true,
            group: 'docs',
            icon,
            preview,
            run: open(),
          },
        ]
        for (const h of page.headings) {
          rows.push({
            key: `mode-docs-${page.path}#${h.id}`,
            label: h.text,
            subtitle: t('modes.docs.inPage', { title: page.title }),
            stacked: true,
            group: 'docs',
            icon,
            preview: { ...preview, path: `/help${page.path}#${h.id}` },
            run: open(h.id),
          })
        }
        return rows
      })
      return {
        header: t('modes.docs.header'),
        items:
          items.length > 0
            ? items
            : hint('mode-docs-none', t('modes.docs.none', { term }), icon),
      }
    }
    if (mode.kind === 'assistant') {
      return {
        header: t('modes.assistant.header'),
        items: [
          {
            key: 'mode-assistant',
            label:
              term === ''
                ? t('modes.assistant.empty')
                : t('modes.assistant.ask', { term }),
            group: 'assistant',
            icon: <BotGlassIcon size="1.4em" />,
            preview: {
              kind: 'assistant',
              description: t('modes.assistant.preview'),
              primaryLabel: t('modes.assistant.open'),
            },
            run: () => void navigate({ to: '/ai-assistant' }),
          },
        ],
      }
    }
    const icon = <PackageGlassIcon size="1.4em" />
    if (!ready) {
      return {
        header: t('modes.docker.header'),
        items: hint('mode-docker-hint', t('modes.docker.hint'), icon),
      }
    }
    if (images.isPending) {
      return {
        header: t('modes.docker.header'),
        items: hint('mode-docker-loading', t('modes.docker.searching'), icon),
      }
    }
    if (images.isError) {
      return {
        header: t('modes.docker.header'),
        items: hint('mode-docker-error', t('modes.docker.failed'), icon),
      }
    }
    const repos = (images.data ?? []).slice(0, MAX_IMAGES)
    const items: PaletteItem[] = repos.map((r) => ({
      key: `mode-docker-${r.repo_name}`,
      label: r.repo_name,
      subtitle: r.short_description,
      stacked: true,
      group: 'docker',
      preview: {
        kind: 'image',
        description: r.short_description,
        primaryLabel: t('modes.docker.deploy'),
        facts: [
          r.is_official ? t('modes.docker.official') : `${r.star_count} stars`,
        ],
      },
      icon,
      badge: (
        <span className="text-xs text-muted-foreground">
          {r.is_official ? t('modes.docker.official') : `★ ${r.star_count}`}
        </span>
      ),
      run: () =>
        void navigate({
          to: '/apps/new',
          search: { deploy: '1', image: r.repo_name },
        }),
    }))
    return {
      header: t('modes.docker.header'),
      items:
        items.length > 0
          ? items
          : hint('mode-docker-none', t('modes.docker.none', { term }), icon),
    }
  }, [
    mode,
    term,
    debounced,
    ready,
    docs.data,
    images.data,
    images.isPending,
    images.isError,
    navigate,
    t,
  ])
}
