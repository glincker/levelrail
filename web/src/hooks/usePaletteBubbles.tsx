import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { RestoreGlassIcon } from '@thesvg/ui-react'
import { nextUp, topFrecent } from '@/lib/paletteHistory'
import type { PaletteItem } from '../components/commandPaletteData'
import type { BubbleGroup } from '../components/palette/RecentBubbles'

const MAX_RECENT = 6
const MAX_MOST_USED = 5
const MAX_NEXT = 3

export interface BubbleSource {
  group: BubbleGroup
  items: Map<string, PaletteItem>
}

interface Input {
  baseItems: PaletteItem[]
  recentKeys: string[]
  recentSearches: string[]
  /** Bumped whenever the stored history changes, so the bubbles re-read it. */
  version: number
  applyQuery: (q: string) => void
  onRemoveKey: (key: string) => void
  onRemoveSearch: (q: string) => void
  /** Runs an item the way a result row would, so it is recorded and the palette closes. */
  select: (item: PaletteItem) => void
}

// Recent pages and searches, the most used destinations and what you usually open next.
export function usePaletteBubbles(input: Input): BubbleGroup[] {
  const { t } = useTranslation('palette')
  const { baseItems, recentKeys, recentSearches, version } = input
  const { applyQuery, onRemoveKey, onRemoveSearch, select } = input

  return React.useMemo<BubbleGroup[]>(() => {
    void version
    const byKey = new Map(baseItems.map((i) => [i.key, i]))
    const seen = new Set<string>()
    const page = (key: string, removable: boolean) => {
      const item = byKey.get(key)
      if (!item || seen.has(key)) return []
      seen.add(key)
      return [
        {
          key: `bubble-${key}`,
          label: item.label,
          icon: item.icon,
          onPick: () => {
            select(item)
          },
          onRemove: removable
            ? () => {
                onRemoveKey(key)
              }
            : undefined,
          removeLabel: t('bubbles.remove', { name: item.label }),
        },
      ]
    }
    const recentPages = recentKeys
      .flatMap((k) => page(k, true))
      .slice(0, MAX_RECENT)
    const searches = recentSearches.slice(0, MAX_RECENT).map((s) => ({
      key: `bubble-search-${s}`,
      label: s,
      icon: <RestoreGlassIcon size="1.2em" />,
      onPick: () => {
        applyQuery(s)
      },
      onRemove: () => {
        onRemoveSearch(s)
      },
      removeLabel: t('bubbles.remove', { name: s }),
    }))
    const most = topFrecent(MAX_MOST_USED + MAX_RECENT)
      .flatMap((k) => page(k, false))
      .slice(0, MAX_MOST_USED)
    const next = nextUp(MAX_NEXT + MAX_RECENT)
      .flatMap((k) => page(k, false))
      .slice(0, MAX_NEXT)
    return [
      {
        id: 'recent',
        label: t('bubbles.recent'),
        bubbles: [...recentPages, ...searches],
      },
      { id: 'most', label: t('bubbles.mostUsed'), bubbles: most },
      { id: 'next', label: t('bubbles.nextUp'), bubbles: next },
    ]
  }, [
    baseItems,
    recentKeys,
    recentSearches,
    version,
    applyQuery,
    onRemoveKey,
    onRemoveSearch,
    select,
    t,
  ])
}
