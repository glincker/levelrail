import * as React from 'react'
import { PinGlassIcon } from '@thesvg/ui-react'
import { topFrecent } from '@/lib/paletteHistory'
import { PALETTE_BUBBLE_MAX } from '@/lib/paletteConfig'
import type { PaletteItem } from '../components/commandPaletteData'
import type { Bubble } from '../components/palette/RecentBubbles'

interface Pinned {
  path: string
  title: string
}

interface Input {
  baseItems: PaletteItem[]
  recentKeys: string[]
  pinned: readonly Pinned[]
  /** Bumped whenever the stored history changes, so the bubbles re-read it. */
  version: number
  openPath: (path: string) => void
  /** Runs an item the way a result row would, so it is recorded and the palette closes. */
  select: (item: PaletteItem) => void
}

// Quick links beside the search pill: most visited first, then recently opened, then pinned.
export function usePaletteBubbles(input: Input): Bubble[] {
  const { baseItems, recentKeys, pinned, version, openPath, select } = input
  return React.useMemo<Bubble[]>(() => {
    void version
    const byKey = new Map(baseItems.map((i) => [i.key, i]))
    const seen = new Set<string>()
    const out: Bubble[] = []
    const add = (b: Bubble) => {
      if (out.length >= PALETTE_BUBBLE_MAX || seen.has(b.key)) return
      seen.add(b.key)
      out.push(b)
    }
    const page = (key: string) => {
      const item = byKey.get(key)
      if (!item) return
      add({
        key: `bubble-${key}`,
        label: item.label,
        icon: item.icon,
        onPick: () => select(item),
      })
    }
    topFrecent(PALETTE_BUBBLE_MAX * 2).forEach(page)
    recentKeys.forEach(page)
    for (const p of pinned) {
      add({
        key: `bubble-pin-${p.path}`,
        label: p.title,
        icon: <PinGlassIcon size="1.4em" />,
        onPick: () => openPath(p.path),
      })
    }
    return out
  }, [baseItems, recentKeys, pinned, version, openPath, select])
}
