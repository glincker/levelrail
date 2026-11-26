import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { CodeBlock } from '@/components/ui/code-block'
import { cn } from '@/lib/utils'
import { parsePlan, type PlanNode } from '../../lib/sqlConsole/plan'

// The explain route returns text, so the tree is parsed from it. If no
// line carries a cost the raw text is shown instead.
export function ExplainTree({ lines }: { lines: string[] }) {
  const { t } = useTranslation('databases')
  const tree = useMemo(() => parsePlan(lines), [lines])
  const [raw, setRaw] = useState(false)
  const text = lines.join('\n')

  if (!tree) {
    return (
      <div className="space-y-1">
        <p className="text-xs text-muted-foreground">
          {t('viewer.console.explainTree.unparsed')}
        </p>
        <CodeBlock code={text} maxHeight="lg" />
      </div>
    )
  }
  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between gap-2">
        <p className="text-xs font-medium text-muted-foreground">
          {t('viewer.console.plan')}
        </p>
        <Button
          type="button"
          size="sm"
          variant="ghost"
          aria-pressed={raw}
          onClick={() => {
            setRaw((r) => !r)
          }}
        >
          {raw
            ? t('viewer.console.explainTree.showTree')
            : t('viewer.console.explainTree.showRaw')}
        </Button>
      </div>
      {raw ? (
        <CodeBlock code={text} maxHeight="lg" />
      ) : (
        <ul className="space-y-1 text-sm">
          {tree.map((n, i) => (
            <PlanItem key={`${n.label}-${String(i)}`} node={n} root />
          ))}
        </ul>
      )}
    </div>
  )
}

function PlanItem({ node, root }: { node: PlanNode; root?: boolean }) {
  const { t } = useTranslation('databases')
  return (
    <li className={cn(root ? '' : 'ml-4 border-l border-border pl-3')}>
      <div
        className={cn(
          'rounded-md border px-2 py-1',
          node.hottest
            ? 'border-tone-warning-border bg-tone-warning-soft'
            : 'border-transparent',
        )}
      >
        <div className="flex flex-wrap items-baseline gap-x-3 gap-y-0.5">
          <span className="font-mono text-xs font-medium">{node.label}</span>
          {node.hottest ? (
            <span className="rounded bg-tone-warning-soft px-1 text-xs font-medium text-tone-warning">
              {t('viewer.console.explainTree.hottest')}
            </span>
          ) : null}
          <span className="text-xs text-muted-foreground">
            {node.totalCost !== null
              ? t('viewer.console.explainTree.cost', {
                  value: node.totalCost.toLocaleString(),
                })
              : ''}
            {node.rows !== null
              ? ` | ${t('viewer.console.explainTree.rows', { count: node.rows })}`
              : ''}
            {node.actualMs !== null
              ? ` | ${t('viewer.console.explainTree.time', { ms: node.actualMs })}`
              : ''}
          </span>
        </div>
        {node.details.length > 0 ? (
          <p className="break-words font-mono text-xs text-muted-foreground">
            {node.details.join(' | ')}
          </p>
        ) : null}
      </div>
      {node.children.length > 0 ? (
        <ul className="mt-1 space-y-1">
          {node.children.map((c, i) => (
            <PlanItem key={`${c.label}-${String(i)}`} node={c} />
          ))}
        </ul>
      ) : null}
    </li>
  )
}
