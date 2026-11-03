import { PlusGlassIcon, XGlassIcon } from '@thesvg/ui-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { newTaintRow, TAINT_EFFECTS, type TaintRow } from '../../lib/taintRows'
import type { TaintEffect, TolerationOperator } from '../../types/nodeMoves'

const SELECT_CLASS =
  'rounded-md border border-input bg-background px-2 py-1.5 text-xs text-foreground'

// Horizontal key / value / effect rows shared by node taints and app
// tolerations; tolerations add an operator select.
export function TaintRowsEditor({
  rows,
  onChange,
  withOperator,
  disabled,
  addLabel,
}: {
  rows: TaintRow[]
  onChange: (rows: TaintRow[]) => void
  withOperator?: boolean
  disabled?: boolean
  addLabel: string
}) {
  const { t } = useTranslation('nodes')
  function update(id: number, patch: Partial<TaintRow>) {
    onChange(rows.map((r) => (r.id === id ? { ...r, ...patch } : r)))
  }

  return (
    <div className="space-y-2">
      {rows.map((row) => (
        <div key={row.id} className="flex flex-wrap items-center gap-2">
          <Input
            value={row.key}
            placeholder={t('taints.keyPlaceholder')}
            aria-label={t('taints.keyPlaceholder')}
            disabled={disabled}
            onChange={(e) => update(row.id, { key: e.target.value })}
            className="w-40 font-mono text-xs"
          />
          {withOperator ? (
            <select
              className={SELECT_CLASS}
              aria-label={t('taints.operator')}
              disabled={disabled}
              value={row.operator}
              onChange={(e) =>
                update(row.id, {
                  operator: e.target.value as TolerationOperator,
                })
              }
            >
              <option value="Equal">{t('taints.operatorEqual')}</option>
              <option value="Exists">{t('taints.operatorExists')}</option>
            </select>
          ) : (
            <span className="text-muted-foreground" aria-hidden="true">
              =
            </span>
          )}
          <Input
            value={row.operator === 'Exists' ? '' : row.value}
            placeholder={t('taints.valuePlaceholder')}
            aria-label={t('taints.valuePlaceholder')}
            disabled={disabled || row.operator === 'Exists'}
            onChange={(e) => update(row.id, { value: e.target.value })}
            className="w-40 font-mono text-xs"
          />
          <select
            className={SELECT_CLASS}
            aria-label={t('taints.effect')}
            disabled={disabled}
            value={row.effect}
            onChange={(e) =>
              update(row.id, { effect: e.target.value as TaintEffect })
            }
          >
            {TAINT_EFFECTS.map((effect) => (
              <option key={effect} value={effect}>
                {effect}
              </option>
            ))}
          </select>
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            disabled={disabled}
            aria-label={t('taints.remove', { key: row.key })}
            onClick={() => onChange(rows.filter((r) => r.id !== row.id))}
          >
            <XGlassIcon size="1.4em" />
          </Button>
        </div>
      ))}
      <Button
        type="button"
        variant="outline"
        size="sm"
        disabled={disabled}
        onClick={() => onChange([...rows, newTaintRow()])}
      >
        <PlusGlassIcon size="1.4em" />
        {addLabel}
      </Button>
    </div>
  )
}
