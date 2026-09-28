import { Link } from '@tanstack/react-router'
import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import type {
  ImportItemStatus,
  PlatformImportItem,
} from '../queries/platformImport'
import { isSelectable, itemKey } from '../lib/platformImport'

const statusLabel: Record<ImportItemStatus, string> = {
  mapped: 'Ready',
  'needs-attention': 'Needs attention',
  unsupported: 'Not supported',
  'already-imported': 'Already imported',
  skipped: 'Skipped',
  created: 'Created',
  failed: 'Failed',
}

const statusVariant: Record<
  ImportItemStatus,
  'success' | 'warning' | 'destructive' | 'muted'
> = {
  mapped: 'success',
  'needs-attention': 'warning',
  unsupported: 'destructive',
  'already-imported': 'muted',
  skipped: 'muted',
  created: 'success',
  failed: 'destructive',
}

function ItemLink({ item }: { item: PlatformImportItem }) {
  if (item.status !== 'created' && item.status !== 'already-imported') {
    return null
  }
  if (!item.target) return null
  if (item.kind === 'app') {
    return (
      <Link
        to="/apps/$name"
        params={{ name: item.target }}
        className="text-sm text-primary underline-offset-4 hover:underline"
      >
        Open {item.target}
      </Link>
    )
  }
  if (item.kind === 'database') {
    return (
      <Link
        to="/databases/$name"
        params={{ name: item.target }}
        className="text-sm text-primary underline-offset-4 hover:underline"
      >
        Open {item.target}
      </Link>
    )
  }
  return null
}

interface Props {
  items: PlatformImportItem[]
  selected?: ReadonlySet<string>
  onToggle?: (key: string, next: boolean) => void
}

// One row per discovered resource. Checkboxes only appear in the review
// step (when onToggle is given); the final summary reuses this list
// read-only with links to what was created.
export function PlatformImportReportList({ items, selected, onToggle }: Props) {
  return (
    <ul className="divide-y divide-border rounded-lg border border-border">
      {items.map((item) => {
        const key = itemKey(item)
        const reasons = item.reasons ?? []
        const manual = item.manual ?? []
        return (
          <li key={key} className="flex gap-3 p-3">
            {onToggle ? (
              <Checkbox
                aria-label={`Import ${item.source_name}`}
                checked={selected?.has(key) ?? false}
                disabled={!isSelectable(item)}
                onCheckedChange={(next) => onToggle(key, next === true)}
                className="mt-1"
              />
            ) : null}
            <div className="min-w-0 flex-1 space-y-1">
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-medium text-foreground">
                  {item.source_name}
                </span>
                <Badge variant="outline">{item.kind}</Badge>
                <Badge variant={statusVariant[item.status]}>
                  {statusLabel[item.status]}
                </Badge>
                {item.target && item.target !== item.source_name ? (
                  <span className="text-xs text-muted-foreground">
                    as {item.target}
                  </span>
                ) : null}
              </div>
              {reasons.length > 0 ? (
                <ul className="list-disc space-y-0.5 pl-4 text-sm text-muted-foreground">
                  {reasons.map((r) => (
                    <li key={r}>{r}</li>
                  ))}
                </ul>
              ) : null}
              {manual.length > 0 ? (
                <ul className="space-y-0.5 text-sm text-foreground">
                  {manual.map((m) => (
                    <li key={m}>
                      <span className="font-medium">Next: </span>
                      {m}
                    </li>
                  ))}
                </ul>
              ) : null}
              <ItemLink item={item} />
            </div>
          </li>
        )
      })}
    </ul>
  )
}
