import { useMemo, useState } from 'react'
import { useRouterState } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ArrowDownGlassIcon,
  ArrowUpGlassIcon,
  GripVerticalGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Switch } from '@/components/ui/switch'
import { useRouteAvailable } from '@/lib/routeAvailability'
import {
  applyNavPrefs,
  canHideGroup,
  canHideItem,
  groupHasItem,
  moveId,
  moveIdTo,
  orderIds,
  withGroupHidden,
  withGroupOrder,
  withItemHidden,
  withItemOrder,
} from '@/lib/navPrefs'
import { cn } from '@/lib/utils'
import {
  activeGlobalItemId,
  visibleGlobalFooter,
  visibleGlobalGroups,
} from './navModel'
import { resetNavPersonalization, useNavPrefs } from './useNavPrefs'

const ICON_BTN =
  'inline-flex size-8 shrink-0 cursor-pointer items-center justify-center rounded-md text-muted-foreground outline-hidden hover:bg-state-accent/10 hover:text-foreground focus-visible:ring-2 focus-visible:ring-sidebar-ring disabled:cursor-not-allowed disabled:opacity-40 pointer-coarse:size-10 [&_svg]:size-4'

interface Row {
  id: string
  label: string
  canHide: boolean
  hidden: boolean
}

function ReorderRow({
  row,
  index,
  total,
  hideReason,
  onMove,
  onHidden,
  onDrop,
  dragId,
  setDragId,
  indent,
}: {
  row: Row
  index: number
  total: number
  hideReason: string
  onMove: (id: string, delta: -1 | 1) => void
  onHidden: (id: string, hidden: boolean) => void
  onDrop: (from: string, to: string) => void
  dragId: string | null
  setDragId: (id: string | null) => void
  indent?: boolean
}) {
  const { t } = useTranslation()
  const reasonId = `customize-reason-${row.id}`
  return (
    <li
      data-testid={`customize-row-${row.id}`}
      onDragOver={(e) => {
        if (dragId) e.preventDefault()
      }}
      onDrop={(e) => {
        e.preventDefault()
        if (dragId) onDrop(dragId, row.id)
        setDragId(null)
      }}
      className={cn(
        'flex min-h-10 items-center gap-1 rounded-md px-1',
        indent && 'ml-4',
        dragId === row.id && 'opacity-50',
      )}
    >
      <span
        draggable
        aria-hidden="true"
        onDragStart={(e) => {
          e.dataTransfer.effectAllowed = 'move'
          setDragId(row.id)
        }}
        onDragEnd={() => setDragId(null)}
        className="inline-flex size-8 shrink-0 cursor-grab items-center justify-center text-muted-foreground active:cursor-grabbing pointer-coarse:size-10"
      >
        <GripVerticalGlassIcon className="size-4" />
      </span>
      <span
        className={cn(
          'min-w-0 flex-1 truncate text-sm',
          row.hidden && 'text-muted-foreground',
        )}
      >
        {row.label}
      </span>
      {!row.canHide ? (
        <span id={reasonId} className="shrink-0 text-xs text-muted-foreground">
          {hideReason}
        </span>
      ) : null}
      <button
        type="button"
        className={ICON_BTN}
        disabled={index === 0}
        aria-label={t('shell.customize.moveUp', { label: row.label })}
        onClick={() => onMove(row.id, -1)}
      >
        <ArrowUpGlassIcon aria-hidden="true" />
      </button>
      <button
        type="button"
        className={ICON_BTN}
        disabled={index === total - 1}
        aria-label={t('shell.customize.moveDown', { label: row.label })}
        onClick={() => onMove(row.id, 1)}
      >
        <ArrowDownGlassIcon aria-hidden="true" />
      </button>
      <Switch
        size="sm"
        checked={!row.hidden}
        disabled={!row.canHide && !row.hidden}
        aria-label={t('shell.customize.show', { label: row.label })}
        aria-describedby={!row.canHide ? reasonId : undefined}
        onCheckedChange={(checked) => onHidden(row.id, !checked)}
      />
    </li>
  )
}

export function SidebarCustomizeDialog({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const routeAvailable = useRouteAvailable()
  const { prefs, update } = useNavPrefs()
  const [announce, setAnnounce] = useState('')
  const [dragId, setDragId] = useState<string | null>(null)

  const base = useMemo(
    () => visibleGlobalGroups(routeAvailable),
    [routeAvailable],
  )
  const activeId = activeGlobalItemId(pathname, [
    ...base.flatMap((g) => g.items),
    ...visibleGlobalFooter(),
  ])
  // Dialog lists every item (hidden ones too); only order comes from prefs.
  const ordered = applyNavPrefs(
    base,
    { ...prefs, hiddenItems: [], hiddenGroups: [] },
    activeId,
  )
  const groups = ordered
  const groupLabel = (id: string, fallback: string) =>
    t(`shell.navGroup.${id}`, { defaultValue: fallback })
  const itemLabel = (id: string, fallback: string) =>
    t(`shell.nav.${id}`, { defaultValue: fallback })
  const groupIds = orderIds(
    groups.filter((g) => !g.ungrouped).map((g) => g.id),
    prefs.groupOrder,
  )
  const listIds = [
    ...groups.filter((g) => g.ungrouped).map((g) => g.id),
    ...groupIds,
  ]

  const say = (label: string, ids: string[], id: string) =>
    setAnnounce(
      t('shell.customize.moved', {
        label,
        position: ids.indexOf(id) + 1,
        total: ids.length,
      }),
    )

  const moveGroup = (id: string, delta: -1 | 1) => {
    const next = moveId(groupIds, id, delta)
    update((p) => withGroupOrder(p, next))
    const g = groups.find((x) => x.id === id)
    if (g) say(groupLabel(g.id, g.label), next, id)
  }
  const dropGroup = (from: string, to: string) =>
    update((p) => withGroupOrder(p, moveIdTo(groupIds, from, to)))

  const moveItem = (groupId: string, id: string, delta: -1 | 1) => {
    const g = groups.find((x) => x.id === groupId)
    if (!g) return
    const next = moveId(
      g.items.map((i) => i.id),
      id,
      delta,
    )
    update((p) => withItemOrder(p, groupId, next))
    const it = g.items.find((i) => i.id === id)
    if (it) say(itemLabel(it.id, it.label), next, id)
  }
  const dropItem = (groupId: string, from: string, to: string) => {
    const g = groups.find((x) => x.id === groupId)
    if (!g) return
    const ids = g.items.map((i) => i.id)
    if (!ids.includes(from)) return
    update((p) => withItemOrder(p, groupId, moveIdTo(ids, from, to)))
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('shell.customize.title')}</DialogTitle>
          <DialogDescription>
            {t('shell.customize.description')}
          </DialogDescription>
        </DialogHeader>
        <div className="max-h-[60vh] overflow-y-auto pr-1">
          <ul aria-label={t('shell.customize.groups')} className="space-y-3">
            {listIds.map((gid) => {
              const g = groups.find((x) => x.id === gid)
              if (!g) return null
              const label = groupLabel(g.id, g.label)
              const groupRow: Row = {
                id: g.id,
                label,
                canHide: canHideGroup(g, activeId),
                hidden: prefs.hiddenGroups.includes(g.id),
              }
              const gi = groupIds.indexOf(g.id)
              const hideReason = groupHasItem(g, activeId)
                ? t('shell.customize.currentGroup')
                : ''
              return (
                <li key={g.id}>
                  <ul className="space-y-0.5">
                    {g.ungrouped ? null : (
                      <ReorderRow
                        row={groupRow}
                        index={gi}
                        total={groupIds.length}
                        hideReason={hideReason}
                        onMove={moveGroup}
                        onHidden={(id, hidden) =>
                          update((p) => withGroupHidden(p, id, hidden))
                        }
                        onDrop={dropGroup}
                        dragId={dragId}
                        setDragId={setDragId}
                      />
                    )}
                    <li>
                      <ul
                        aria-label={t('shell.customize.itemsIn', { label })}
                        className="space-y-0.5"
                      >
                        {g.items.map((item, ii) => {
                          const row: Row = {
                            id: item.id,
                            label: itemLabel(item.id, item.label),
                            canHide: canHideItem(item.id, activeId),
                            hidden: prefs.hiddenItems.includes(item.id),
                          }
                          return (
                            <ReorderRow
                              key={item.id}
                              row={row}
                              index={ii}
                              total={g.items.length}
                              hideReason={t('shell.customize.currentPage')}
                              onMove={(id, d) => moveItem(g.id, id, d)}
                              onHidden={(id, hidden) =>
                                update((p) =>
                                  withItemHidden(p, id, hidden, activeId),
                                )
                              }
                              onDrop={(from, to) => dropItem(g.id, from, to)}
                              dragId={dragId}
                              setDragId={setDragId}
                              indent={!g.ungrouped}
                            />
                          )
                        })}
                      </ul>
                    </li>
                  </ul>
                </li>
              )
            })}
          </ul>
        </div>
        <p role="status" aria-live="polite" className="sr-only">
          {announce}
        </p>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              resetNavPersonalization()
              setAnnounce(t('shell.customize.resetDone'))
            }}
          >
            {t('shell.customize.reset')}
          </Button>
          <Button type="button" onClick={() => onOpenChange(false)}>
            {t('shell.customize.done')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
