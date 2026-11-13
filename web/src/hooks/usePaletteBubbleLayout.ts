import * as React from 'react'
import { useTranslation } from 'react-i18next'
import {
  loadPinned,
  pinPrefix,
  savePinned,
  type PinnedBubble,
} from '@/lib/paletteBubbleLayout'
import type { Bubble } from '../components/palette/RecentBubbles'

/** The hand-placed bubbles: pinned set, order, and the live-region message for changes. */
export function usePaletteBubbleLayout() {
  const { t } = useTranslation('palette')
  const [pinned, setPinned] = React.useState<PinnedBubble[]>(loadPinned)
  const [announcement, setAnnouncement] = React.useState('')

  const commit = React.useCallback((next: PinnedBubble[]) => {
    setPinned(next)
    savePinned(next)
  }, [])

  const place = React.useCallback(
    (order: PinnedBubble[], movedId: string, msg: string) => {
      const next = pinPrefix(order, new Set(pinned.map((p) => p.id)), movedId)
      if (!next) {
        setAnnouncement(t('bubbles.limit'))
        return
      }
      commit(next)
      if (msg) setAnnouncement(msg)
    },
    [pinned, commit, t],
  )

  /** The row was reordered; `bubbles` is the new display order. */
  const reorder = React.useCallback(
    (bubbles: readonly Bubble[], movedKey: string) => {
      const order = bubbles.flatMap((b) => (b.entry ? [b.entry] : []))
      place(order, movedKey, '')
    },
    [place],
  )

  /** Pin `entry` so it lands at `index` of the displayed row. */
  const pinAt = React.useCallback(
    (entry: PinnedBubble, index: number, bubbles: readonly Bubble[]) => {
      const rest = bubbles.flatMap((b) =>
        b.entry && b.entry.id !== entry.id ? [b.entry] : [],
      )
      rest.splice(Math.min(index, rest.length), 0, entry)
      place(rest, entry.id, t('bubbles.pinned', { title: entry.title }))
    },
    [place, t],
  )

  const unpin = React.useCallback(
    (id: string) => {
      const found = pinned.find((p) => p.id === id)
      commit(pinned.filter((p) => p.id !== id))
      if (found) setAnnouncement(t('bubbles.unpinned', { title: found.title }))
    },
    [pinned, commit, t],
  )

  const toggle = React.useCallback(
    (entry: PinnedBubble, bubbles: readonly Bubble[]) => {
      if (pinned.some((p) => p.id === entry.id)) unpin(entry.id)
      else pinAt(entry, pinned.length, bubbles)
    },
    [pinned, unpin, pinAt],
  )

  return {
    pinned,
    announcement,
    say: setAnnouncement,
    reorder,
    pinAt,
    unpin,
    toggle,
  }
}
