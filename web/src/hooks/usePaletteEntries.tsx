import * as React from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { useNavigate } from '@tanstack/react-router'
import {
  BotGlassIcon,
  FunnelGlassIcon,
  LayersGlassIcon,
  RestoreGlassIcon,
} from '@thesvg/ui-react'
import { toast } from '@/components/ui/toast'
import { parseMode } from '@/lib/paletteMode'
import { usePaletteModeItems } from './usePaletteModes'
import { rankItems, relatedFor } from '@/lib/paletteSearch'
import { STARTER_KEYS } from '@/lib/paletteSearchIndex'
import { groupHits } from '@/lib/paletteGroups'
import { KIND_VALUES, scopeKeyCompletions, type Chip } from '@/lib/paletteQuery'
import { isFeatureVisible } from '../lib/experimental'
import { appListQueryOptions } from '../queries/apps'
import { useTemplatePaletteItems } from '../components/palette/useTemplatePaletteItems'
import {
  useDeleteSavedSearch,
  useSavedSearches,
  type SearchHit,
} from '../queries/search'
import {
  DEFAULT_QUICK_ACTION_KEYS,
  GROUP_ORDER,
  type PaletteItem,
} from '../components/commandPaletteData'
import type { PaletteEntry } from '../components/paletteList'
import { hitToItem, kindIcon } from '../components/paletteHitItems'
import { buildPaletteSuggestions } from '../components/shell/paletteSuggestions'
import { useExperimentalFeatures } from './useExperimental'
import { usePaletteAppActions } from './usePaletteAppActions'
import type { usePaletteSearch } from './usePaletteSearch'

const STATIC_SEARCH_GROUPS = new Set([
  'Actions',
  'Navigate',
  'Settings',
  'Domain actions',
])
const MAX_STATIC_MATCHES = 5
const MAX_VALUE_SUGGESTIONS = 5

function bucket(items: readonly PaletteItem[]): Map<string, PaletteItem[]> {
  const byGroup = new Map<string, PaletteItem[]>()
  for (const item of items) {
    byGroup.set(item.group, [...(byGroup.get(item.group) ?? []), item])
  }
  return byGroup
}

interface Input {
  ps: ReturnType<typeof usePaletteSearch>
  baseItems: PaletteItem[]
  recentKeys: string[]
  recentSearches: string[]
  expanded: ReadonlySet<string>
  expand: (kind: string) => void
  currentApp?: string
}

// Builds the flat header/item list the palette renders, for both the idle and the searching state.
export function usePaletteEntries(input: Input): PaletteEntry[] {
  const { ps, baseItems, recentKeys, recentSearches, expanded, expand } = input
  const { currentApp } = input
  const { t } = useTranslation('palette')
  const navigate = useNavigate()
  const experimental = useExperimentalFeatures()
  const { restartApp } = usePaletteAppActions()
  const saved = useSavedSearches()
  const queryClient = useQueryClient()
  const deleteSavedSearch = useDeleteSavedSearch().mutate

  const mode = React.useMemo(() => parseMode(ps.text), [ps.text])
  const modeItems = usePaletteModeItems(mode, ps.setInput)
  const searching = ps.serverQuery !== ''
  const failed = searching && ps.search.isError
  const hits = React.useMemo<SearchHit[]>(
    () => (searching && !ps.search.isError ? (ps.search.data?.hits ?? []) : []),
    [searching, ps.search.isError, ps.search.data],
  )
  const savedRows = saved.data
  const related = React.useMemo(() => relatedFor(ps.text), [ps.text])
  const templateItems = useTemplatePaletteItems(
    ps.text,
    ps.text.trim() !== '' && ps.chips.length === 0,
    related.templates,
  )

  const looseNavigate = React.useCallback(
    (opts: { to: string; search?: Record<string, string> }) =>
      void navigate(opts),
    [navigate],
  )

  return React.useMemo<PaletteEntry[]>(() => {
    const out: PaletteEntry[] = []
    let index = 0
    const push = (key: string, label: string, items: PaletteItem[]) => {
      if (items.length === 0) return
      out.push({ type: 'header', key: `h-${key}`, label })
      for (const item of items) {
        out.push({ type: 'item', key: item.key, item, index: index++ })
      }
    }
    const groupLabel = (g: string) =>
      t(`groups.${g}` as 'groups.Actions', { defaultValue: g })
    const pushStatic = (items: PaletteItem[]) => {
      const byGroup = bucket(items)
      for (const g of GROUP_ORDER) {
        push(`base-${g}`, groupLabel(g), byGroup.get(g) ?? [])
      }
    }
    const q = ps.text.trim()

    if (modeItems) {
      push(`mode-${mode?.kind ?? ''}`, modeItems.header, modeItems.items)
      return out
    }

    const tok = ps.token
    if (tok) {
      const head = ps.text.slice(0, tok.start)
      const filters: PaletteItem[] = scopeKeyCompletions(tok.partialKey).map(
        (key) => ({
          key: `suggest-scope-${key}`,
          label: `${key}:`,
          group: 'filters',
          icon: <FunnelGlassIcon size="1.4em" />,
          keepOpen: true,
          run: () => ps.setInput(`${head}${key}:`),
        }),
      )
      if (tok.key) {
        const key = tok.key
        const want = tok.value.toLowerCase()
        const values: string[] =
          key === 'kind'
            ? KIND_VALUES.filter((k) => k.startsWith(want))
            : ps.suggestions
                .filter((s) => s.kind === (key === 'env' ? 'environment' : key))
                .map((s) => s.text)
        for (const value of values.slice(0, MAX_VALUE_SUGGESTIONS)) {
          const chip: Chip = { key, value }
          filters.push({
            key: `suggest-value-${key}-${value}`,
            label: `${key}:${value}`,
            group: 'filters',
            icon: <FunnelGlassIcon size="1.4em" />,
            keepOpen: true,
            run: () => ps.acceptScope(chip),
          })
        }
      }
      push('filters', t('groups.filters'), filters)
    }

    if (!searching) {
      const byKey = new Map(baseItems.map((i) => [i.key, i]))
      const specs = buildPaletteSuggestions({
        currentApp: currentApp ? decodeURIComponent(currentApp) : undefined,
        apps: [],
        recentKeys,
        assistantEnabled: isFeatureVisible('ai-chat', experimental),
      }).filter((sp) => sp.kind !== 'app-action')
      const suggested: PaletteItem[] = specs.map((sp) => ({
        key: sp.key,
        label: sp.label,
        group: 'Suggested',
        icon:
          sp.kind === 'assistant' ? (
            <BotGlassIcon size="1.4em" />
          ) : (
            <LayersGlassIcon size="1.4em" />
          ),
        run: () =>
          void navigate(
            sp.kind === 'assistant'
              ? { to: '/ai-assistant' }
              : { to: '/apps/$name', params: { name: sp.app ?? '' } },
          ),
      }))
      // Cache-only read: opening the palette must not fire a list request.
      const cachedApps = queryClient.getQueryData(
        appListQueryOptions().queryKey,
      )
      const starter =
        cachedApps === undefined
          ? undefined
          : cachedApps.length === 0
            ? STARTER_KEYS.noApps
            : STARTER_KEYS.hasApps
      const quick = (starter ?? DEFAULT_QUICK_ACTION_KEYS).flatMap((k) => {
        const item = byKey.get(k)
        if (!item) return []
        const why = starter
          ? t(`preview.starter.${k}` as 'preview.starter.nav-backups', {
              defaultValue: '',
            }) || undefined
          : undefined
        return [
          {
            ...item,
            key: `suggested-quick-${item.key}`,
            group: 'Suggested',
            subtitle: why ?? item.subtitle,
            preview: {
              kind: 'page' as const,
              ...item.preview,
              description: why ?? item.preview?.description,
            },
          },
        ]
      })
      push('Suggested', groupLabel('Suggested'), [...suggested, ...quick])
      push(
        'Recent',
        groupLabel('Recent'),
        recentKeys.flatMap((k) => {
          const item = byKey.get(k)
          return item
            ? [{ ...item, key: `recent-${item.key}`, group: 'Recent' }]
            : []
        }),
      )
      push(
        'recentSearches',
        t('groups.recentSearches'),
        recentSearches.map((s) => ({
          key: `recent-search-${s}`,
          label: s,
          group: 'recentSearches',
          icon: <RestoreGlassIcon size="1.4em" />,
          keepOpen: true,
          run: () => ps.applyQuery(s),
        })),
      )
      push(
        'savedSearches',
        t('groups.savedSearches'),
        (savedRows ?? []).map((s) => ({
          key: `saved-search-${s.id}`,
          label: s.name,
          subtitle: s.query,
          group: 'savedSearches',
          icon: kindIcon('saved'),
          keepOpen: true,
          run: () => ps.applyQuery(s.query),
          removeLabel: t('saved.delete', { name: s.name }),
          onRemove: () =>
            deleteSavedSearch(s.id, {
              onSuccess: () =>
                toast.add({
                  title: t('saved.deleted', { name: s.name }),
                  type: 'success',
                }),
              onError: (e) =>
                toast.add({
                  title: t('saved.deleteFailed', { message: e.message }),
                  type: 'error',
                }),
            }),
        })),
      )
      pushStatic(baseItems)
      return out
    }

    const byKey = new Map(baseItems.map((i) => [i.key, i]))
    const relatedItems: PaletteItem[] =
      ps.chips.length > 0
        ? []
        : related.keys.flatMap((k) => {
            const item = byKey.get(k)
            return item
              ? [{ ...item, key: `related-${item.key}`, group: 'Suggested' }]
              : []
          })
    const relatedKeys = new Set(related.keys)
    push('related', groupLabel('Suggested'), relatedItems)

    // Client-only matches: the built-in fallback when search fails, and a
    // supplement to server hits otherwise. Chips mean a scoped search, so
    // static entries would only add noise.
    const clientItems = (hideSettings: boolean): PaletteItem[] => {
      if (ps.chips.length > 0 || q === '') return []
      const counts = new Map<string, number>()
      return rankItems(baseItems, q, (i) => i.keywords).filter((i) => {
        if (!STATIC_SEARCH_GROUPS.has(i.group)) return false
        if (relatedKeys.has(i.key)) return false
        if (hideSettings && i.group === 'Settings') return false
        const n = (counts.get(i.group) ?? 0) + 1
        counts.set(i.group, n)
        return n <= MAX_STATIC_MATCHES
      })
    }

    if (failed) {
      pushStatic(clientItems(false))
      push('Templates', groupLabel('Templates'), templateItems)
      return out
    }

    const applySaved = (hit: SearchHit) => {
      const row = savedRows?.find((s) => `saved:${s.id}` === hit.ref)
      if (row) ps.applyQuery(row.query)
    }
    for (const g of groupHits(hits, expanded)) {
      const items = g.hits.map((h) => ({
        ...hitToItem(h, {
          t,
          navigate: looseNavigate,
          restartApp: (name) => {
            restartApp(name)
            toast.add({ title: t('restarting', { name }), type: 'success' })
          },
          applySaved,
        }),
        group: g.kind,
      }))
      if (g.total > g.hits.length) {
        items.push({
          key: `more-${g.kind}`,
          label: t('showAll', { count: g.total }),
          group: g.kind,
          icon: kindIcon(g.kind),
          keepOpen: true,
          run: () => expand(g.kind),
        })
      }
      push(
        `kind-${g.kind}`,
        t(`kind.${g.kind}` as 'kind.app', { defaultValue: g.kind }),
        items,
      )
    }
    pushStatic(clientItems(hits.some((h) => h.kind === 'setting')))
    const serverTemplates = new Set(
      hits
        .filter((h) => h.kind === 'template')
        .map((h) => h.title.toLowerCase()),
    )
    push(
      'Templates',
      groupLabel('Templates'),
      templateItems.filter((i) => !serverTemplates.has(i.label.toLowerCase())),
    )
    return out
  }, [
    queryClient,
    mode,
    modeItems,
    related,
    templateItems,
    t,
    ps,
    searching,
    failed,
    baseItems,
    currentApp,
    recentKeys,
    recentSearches,
    savedRows,
    deleteSavedSearch,
    experimental,
    navigate,
    hits,
    expanded,
    expand,
    looseNavigate,
    restartApp,
  ])
}
