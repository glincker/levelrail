import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDownIcon } from '@thesvg/ui-react'
import { cn } from '@/lib/utils'
import { buildAppResourceTree } from '@/lib/appResourceTree'
import type { TreeGroup } from '@/lib/appResourceTree'
import type { AppDetail } from '../../types/appDetail'
import type { ReconcileCondition } from '../../types/deploy'
import { PinnedDigestBadge } from '../PinnedDigestBadge'

function Group({ group }: { group: TreeGroup }) {
  const { t } = useTranslation('common')
  const [open, setOpen] = useState(true)
  const expandable = group.leaves.length > 0
  const title = t(`appResourceTree.groups.${group.id}`)
  return (
    <li role="treeitem" aria-expanded={expandable ? open : undefined}>
      <button
        type="button"
        disabled={!expandable}
        onClick={() => setOpen((o) => !o)}
        className="flex items-center gap-1.5 rounded px-1 py-0.5 text-sm text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none enabled:hover:bg-muted"
      >
        <ChevronDownIcon
          size={12}
          aria-hidden="true"
          className={cn(
            'transition-transform motion-reduce:transition-none',
            !expandable && 'invisible',
            expandable && !open && '-rotate-90',
          )}
        />
        <span className="font-medium">{title}</span>
        {group.count !== undefined && (
          <span className="text-xs text-muted-foreground">{group.count}</span>
        )}
      </button>
      {expandable && open && (
        <ul role="group" className="ml-4 border-l border-border pl-3">
          {group.leaves.map((l) => (
            <li
              key={`${l.label}:${l.detail ?? ''}`}
              role="treeitem"
              className="flex flex-wrap items-center gap-2 py-0.5 text-xs"
            >
              <span className="font-mono text-foreground">{l.label}</span>
              {l.detail && (
                <span className="text-muted-foreground">{l.detail}</span>
              )}
              <PinnedDigestBadge digest={l.digest} />
            </li>
          ))}
        </ul>
      )}
    </li>
  )
}

export function AppResourceTree({
  app,
  conditions,
}: {
  app: AppDetail
  conditions: ReconcileCondition[]
}) {
  const { t } = useTranslation('common')
  const [open, setOpen] = useState(true)
  const tree = buildAppResourceTree(app, conditions)
  if (tree.groups.length === 0) return null
  return (
    <section aria-label={t('appResourceTree.title')} className="text-sm">
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen((o) => !o)}
        className="flex items-center gap-1.5 rounded px-1 py-0.5 font-medium text-foreground hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
      >
        <ChevronDownIcon
          size={12}
          aria-hidden="true"
          className={cn(
            'transition-transform motion-reduce:transition-none',
            !open && '-rotate-90',
          )}
        />
        {t('appResourceTree.title')}
        <span className="font-mono text-xs text-muted-foreground">
          {tree.name}
        </span>
      </button>
      {open && (
        <ul
          role="tree"
          aria-label={t('appResourceTree.title')}
          className="mt-1 ml-2 space-y-0.5"
        >
          {tree.groups.map((g) => (
            <Group key={g.id} group={g} />
          ))}
        </ul>
      )}
    </section>
  )
}
