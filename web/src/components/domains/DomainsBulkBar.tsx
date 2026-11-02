import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { XGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import type { SelectAllState } from '../../lib/domainsBatch'

export interface BulkBarAction {
  id: string
  label: string
  icon: ReactNode
  destructive?: boolean
  onClick: () => void
}

// Compact bar above a table; announces the count to screen readers.
export function DomainsBulkBar({
  count,
  actions,
  onClear,
}: {
  count: number
  actions: BulkBarAction[]
  onClear: () => void
}) {
  const { t } = useTranslation('domains')
  if (count === 0) return null
  return (
    <div
      role="region"
      aria-label={t('bulk.barLabel')}
      className="flex flex-wrap items-center gap-2 rounded-lg border border-border bg-muted/40 px-3 py-1.5"
    >
      <span className="text-sm font-medium" role="status">
        {t('bulk.selected', { count })}
      </span>
      <span className="ml-auto flex flex-wrap items-center gap-1.5">
        {actions.map((a) => (
          <Button
            key={a.id}
            size="sm"
            variant={a.destructive ? 'destructive' : 'outline'}
            onClick={a.onClick}
          >
            {a.icon}
            {a.label}
          </Button>
        ))}
        <Button size="sm" variant="ghost" onClick={onClear}>
          <XGlassIcon size="1.4em" aria-hidden="true" />
          {t('bulk.clear')}
        </Button>
      </span>
    </div>
  )
}

// Header and row checkboxes; z-10 keeps them above a row's link overlay.
export function SelectAllBox({
  state,
  disabled,
  onChange,
}: {
  state: SelectAllState
  disabled?: boolean
  onChange: (on: boolean) => void
}) {
  const { t } = useTranslation('domains')
  return (
    <span className="relative z-10 inline-flex">
      <Checkbox
        aria-label={t('bulk.selectAll')}
        checked={state === 'all'}
        indeterminate={state === 'some'}
        disabled={disabled}
        onCheckedChange={(on) => {
          onChange(on)
        }}
      />
    </span>
  )
}

export function RowSelectBox({
  domain,
  checked,
  blocked,
  onChange,
}: {
  domain: string
  checked: boolean
  // Why the row cannot be selected, shown as a tooltip.
  blocked?: string
  onChange: (on: boolean) => void
}) {
  const { t } = useTranslation('domains')
  return (
    <span className="relative z-10 inline-flex" title={blocked}>
      <Checkbox
        aria-label={t('bulk.selectRow', { domain })}
        checked={checked}
        disabled={blocked !== undefined}
        onCheckedChange={(on) => {
          onChange(on)
        }}
      />
    </span>
  )
}
