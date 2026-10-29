import { PlusGlassIcon, XGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { newLabelRow, type LabelRow } from '../../lib/labelRows'

// Editable key=value rows shared by the node labels card and the
// placement selector editor.
export function LabelRowsEditor({
  rows,
  onChange,
  keyPlaceholder,
  valuePlaceholder,
  addLabel,
  removeLabel,
  disabled,
}: {
  rows: LabelRow[]
  onChange: (rows: LabelRow[]) => void
  keyPlaceholder: string
  valuePlaceholder: string
  addLabel: string
  removeLabel: (key: string) => string
  disabled?: boolean
}) {
  function update(id: number, patch: Partial<LabelRow>) {
    onChange(rows.map((r) => (r.id === id ? { ...r, ...patch } : r)))
  }

  return (
    <div className="space-y-2">
      {rows.map((row) => (
        <div key={row.id} className="flex items-center gap-2">
          <Input
            value={row.key}
            placeholder={keyPlaceholder}
            aria-label={keyPlaceholder}
            disabled={disabled}
            onChange={(e) => update(row.id, { key: e.target.value })}
            className="font-mono text-xs"
          />
          <span className="text-muted-foreground" aria-hidden="true">
            =
          </span>
          <Input
            value={row.value}
            placeholder={valuePlaceholder}
            aria-label={valuePlaceholder}
            disabled={disabled}
            onChange={(e) => update(row.id, { value: e.target.value })}
            className="font-mono text-xs"
          />
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            disabled={disabled}
            aria-label={removeLabel(row.key)}
            onClick={() => onChange(rows.filter((r) => r.id !== row.id))}
          >
            <XGlassIcon size="1.2em" />
          </Button>
        </div>
      ))}
      <Button
        type="button"
        variant="outline"
        size="sm"
        disabled={disabled}
        onClick={() => onChange([...rows, newLabelRow()])}
      >
        <PlusGlassIcon size="1.2em" />
        {addLabel}
      </Button>
    </div>
  )
}
