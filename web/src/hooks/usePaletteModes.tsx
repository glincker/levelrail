import * as React from 'react'
import { useQueries, useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  BookOpenGlassIcon,
  BotGlassIcon,
  HashGlassIcon,
  PackageGlassIcon,
} from '@thesvg/ui-react'
import type { PaletteMode } from '@/lib/paletteMode'
import { recordModeUse } from '@/lib/paletteModeUse'
import { brandForRegistryHost, registryHostOf } from '@/lib/registryRef'
import { searchDocsGrouped } from '@/lib/docsSearch'
import { loadDocsManifest } from '@/lib/docsManifestLoader'
import { BrandIcon } from '../components/BrandIcon'
import type { PaletteItem } from '../components/commandPaletteData'
import { dockerHubKeys, fetchDockerHubSearch } from '../queries/dockerhub'
import {
  fetchRegistryCredentialRepositories,
  registryCredentialKeys,
  registryCredentialListQueryOptions,
} from '../queries/registryCredentials'
import { useSearchSuggest } from '../queries/search'
import { useDebouncedValue } from './useDebouncedValue'

const MIN_TERM = 2
const MAX_DOCS = 8
const MAX_IMAGES = 8
const MAX_CONNECTED = 4
const DEBOUNCE_MS = 250

export interface ModeItems {
  header: string
  items: PaletteItem[]
}

// Results for the prefix modes. Docker Hub is only asked when the user typed
// "docker ", "deploy ", "new " or "+", so ordinary searches never leave the server.
export function usePaletteModeItems(
  mode: PaletteMode | null,
  setInput: (value: string) => void,
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

  // Connected registries: each one's repositories are browsed (cached) and filtered by the term.
  const creds = useQuery({
    ...registryCredentialListQueryOptions(),
    enabled: mode?.kind === 'docker',
  })
  const credList = React.useMemo(
    () => (Array.isArray(creds.data) ? creds.data : []),
    [creds.data],
  )
  const looksLikeRef = registryHostOf(term) !== null
  const browseOn = mode?.kind === 'docker' && ready && !looksLikeRef
  const repoLists = useQueries({
    queries: credList.map((c) => ({
      queryKey: registryCredentialKeys.repositories(c.id),
      queryFn: () => fetchRegistryCredentialRepositories(c.id),
      enabled: browseOn,
      staleTime: 60_000,
      retry: false,
    })),
    combine: (results) => results.map((r) => r.data),
  })

  const tagsOn = mode?.kind === 'tags' && debounced.length >= 1
  const tags = useSearchSuggest(`tag:${debounced}`, tagsOn)
  const envs = useSearchSuggest(`env:${debounced}`, tagsOn)

  const built = React.useMemo<ModeItems | null>(() => {
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
    if (mode.kind === 'tags') {
      const icon = <HashGlassIcon size="1.4em" />
      const found = [
        ...(Array.isArray(tags.data) ? tags.data : []).map((x) => ({
          key: 'tag',
          text: x.text,
        })),
        ...(Array.isArray(envs.data) ? envs.data : []).map((x) => ({
          key: 'env',
          text: x.text,
        })),
      ]
      const rows: PaletteItem[] = found.map((x) => ({
        key: `mode-tags-${x.key}-${x.text}`,
        label: `#${x.text}`,
        subtitle: x.key === 'tag' ? t('modes.tags.tag') : t('modes.tags.env'),
        group: 'tags',
        icon,
        keepOpen: true,
        run: () => {
          setInput(`${x.key}:${x.text} `)
        },
      }))
      return {
        header: t('modes.tags.header'),
        items:
          rows.length > 0
            ? rows
            : hint(
                'mode-tags-hint',
                debounced === ''
                  ? t('modes.tags.hint')
                  : t('modes.tags.none', { term }),
                icon,
              ),
      }
    }
    const hubIcon = <BrandIcon name="docker" className="size-6" />
    const iconFor = (host: string) => {
      const brand = brandForRegistryHost(host)
      return brand ? (
        <BrandIcon name={brand} className="size-6" />
      ) : (
        <PackageGlassIcon size="1.4em" />
      )
    }
    const deploy = (image: string) => () =>
      void navigate({ to: '/apps/new', search: { deploy: '1', image } })
    const header = t('modes.docker.header')
    const ref = registryHostOf(term) ? term : null
    if (ref) {
      const host = registryHostOf(ref) ?? ''
      return {
        header,
        items: [
          {
            key: `mode-image-ref-${ref}`,
            label: ref,
            subtitle: t('modes.docker.refHint', { host }),
            stacked: true,
            group: 'docker',
            icon: iconFor(host),
            preview: {
              kind: 'image' as const,
              description: t('modes.docker.refPreview', { ref }),
              primaryLabel: t('modes.docker.deploy'),
              facts: [host],
            },
            run: deploy(ref),
          },
        ],
      }
    }
    if (!ready) {
      return {
        header,
        items: hint('mode-docker-hint', t('modes.docker.hint'), hubIcon),
      }
    }
    const q = debounced.toLowerCase()
    const connected: PaletteItem[] = credList.flatMap((c, i) =>
      (repoLists[i] ?? [])
        .filter((repo) => repo.toLowerCase().includes(q))
        .slice(0, MAX_CONNECTED)
        .map((repo) => {
          const image = `${c.registry_host}/${repo}`
          return {
            key: `mode-image-conn-${c.id}-${repo}`,
            label: image,
            subtitle: c.name,
            stacked: true,
            group: 'docker',
            icon: iconFor(c.registry_host),
            badge: (
              <span className="text-xs text-muted-foreground">
                {t('modes.docker.connected')}
              </span>
            ),
            preview: {
              kind: 'image' as const,
              description: t('modes.docker.connectedPreview', {
                name: c.name,
              }),
              primaryLabel: t('modes.docker.deploy'),
              facts: [c.registry_host],
            },
            run: deploy(image),
          }
        }),
    )
    const hub: PaletteItem[] = (images.data ?? [])
      .slice(0, MAX_IMAGES)
      .map((r) => ({
        key: `mode-docker-${r.repo_name}`,
        label: r.repo_name,
        subtitle: r.short_description,
        stacked: true,
        group: 'docker',
        preview: {
          kind: 'image' as const,
          description: r.short_description,
          primaryLabel: t('modes.docker.deploy'),
          facts: [
            r.is_official
              ? t('modes.docker.official')
              : `${r.star_count} stars`,
          ],
        },
        icon: hubIcon,
        badge: (
          <span className="text-xs text-muted-foreground">
            {r.is_official ? t('modes.docker.official') : `★ ${r.star_count}`}
          </span>
        ),
        run: deploy(r.repo_name),
      }))
    const items = [...connected, ...hub]
    if (items.length > 0) return { header, items }
    if (images.isPending) {
      return {
        header,
        items: hint(
          'mode-docker-loading',
          t('modes.docker.searching'),
          hubIcon,
        ),
      }
    }
    if (images.isError) {
      return {
        header,
        items: hint('mode-docker-error', t('modes.docker.failed'), hubIcon),
      }
    }
    return {
      header,
      items: hint(
        'mode-docker-none',
        t('modes.docker.none', { term }),
        hubIcon,
      ),
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
    credList,
    repoLists,
    tags.data,
    envs.data,
    setInput,
    navigate,
    t,
  ])

  return React.useMemo<ModeItems | null>(() => {
    if (!built || !mode) return built
    const kind = mode.kind
    return {
      ...built,
      items: built.items.map((item) =>
        /-(hint|loading|none|error)$/.test(item.key)
          ? item
          : {
              ...item,
              run: () => {
                recordModeUse(kind)
                item.run()
              },
            },
      ),
    }
  }, [built, mode])
}
