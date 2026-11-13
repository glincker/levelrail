import * as React from 'react'
import { PinGlassIcon } from '@thesvg/ui-react'
import { topFrecent } from '@/lib/paletteHistory'
import { PALETTE_BUBBLE_MAX } from '@/lib/paletteConfig'
import type { PinnedBubble } from '@/lib/paletteBubbleLayout'
import type { PaletteItem } from '../components/commandPaletteData'
import type { Bubble } from '../components/palette/RecentBubbles'

interface Input {
  baseItems: PaletteItem[]
  recentKeys: string[]
  /** Hand-placed bubbles; they come first, in stored order. */
  pinned: readonly PinnedBubble[]
  /** Bumped whenever the stored history changes, so the bubbles re-read it. */
  version: number
  openPath: (path: string) => void
  /** Runs an item the way a result row would, so it is recorded and the palette closes. */
  select: (item: PaletteItem) => void
}

// Quick links beside the search pill: pinned first, then most visited, then recently opened.
export function usePaletteBubbles(input: Input): Bubble[] {
  const { baseItems, recentKeys, pinned, version, openPath, select } = input
  return React.useMemo<Bubble[]>(() => {
    void version
    const byKey = new Map(baseItems.map((i) => [i.key, i]))
    const pinnedIds = new Set(pinned.map((p) => p.id))
    const seen = new Set<string>()
    const out: Bubble[] = []
    const add = (b: Bubble) => {
      if (out.length >= PALETTE_BUBBLE_MAX || seen.has(b.key)) return
      seen.add(b.key)
      out.push(b)
    }
    const fromItem = (item: PaletteItem): Bubble => {
      const id = `item:${item.key}`
      return {
        key: id,
        label: item.label,
        icon: item.icon,
        href: item.preview?.path,
        pinned: pinnedIds.has(id),
        entry: {
          id,
          title: item.label,
          itemKey: item.key,
          path: item.preview?.path,
        },
        onPick: () => select(item),
      }
    }
    for (const p of pinned) {
      const item = p.itemKey ? byKey.get(p.itemKey) : undefined
      const path = p.path
      add(
        item
          ? { ...fromItem(item), key: p.id, pinned: true, entry: p }
          : {
              key: p.id,
              label: p.title,
              icon: <PinGlassIcon size="1.4em" />,
              href: path,
              pinned: true,
              entry: p,
              onPick: () => {
                if (path) openPath(path)
              },
            },
      )
    }
    const auto = (key: string) => {
      const item = byKey.get(key)
      if (item) add(fromItem(item))
    }
    topFrecent(PALETTE_BUBBLE_MAX * 2).forEach(auto)
    recentKeys.forEach(auto)
    return out
  }, [baseItems, recentKeys, pinned, version, openPath, select])
}
