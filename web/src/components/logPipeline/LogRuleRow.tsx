import { useTranslation } from 'react-i18next'
import {
  ArrowDownGlassIcon,
  ArrowUpGlassIcon,
  TrashGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  LOG_RULE_KINDS,
  type LogRule,
  type LogRuleKind,
} from '../../types/logPipeline'

interface Props {
  rule: LogRule
  index: number
  count: number
  onChange: (next: LogRule) => void
  onMove: (delta: -1 | 1) => void
  onRemove: () => void
}

/** LogRuleRow edits one pipeline rule and its position in the list. */
export function LogRuleRow({
  rule,
  index,
  count,
  onChange,
  onMove,
  onRemove,
}: Props) {
  const { t } = useTranslation('logPipeline')
  const n = index + 1
  return (
    <li className="grid grid-cols-1 gap-3 rounded-lg border border-border p-3 md:grid-cols-[auto_minmax(0,1fr)_auto] md:items-end">
      <label className="space-y-1 text-xs text-muted-foreground">
        {t('rules.kind')}
        <select
          value={rule.kind}
          className="block h-8 w-full rounded-lg border border-input bg-transparent px-2 text-sm text-foreground outline-none focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring"
          onChange={(e) => {
            onChange({ ...rule, kind: e.target.value as LogRuleKind })
          }}
        >
          {LOG_RULE_KINDS.map((k) => (
            <option key={k} value={k}>
              {t(`kinds.${k}`)}
            </option>
          ))}
        </select>
      </label>
      <div className="grid gap-3 sm:grid-cols-2">
        <label className="space-y-1 text-xs text-muted-foreground">
          {t('rules.pattern')}
          <Input
            value={rule.pattern}
            className="font-mono"
            onChange={(e) => {
              onChange({ ...rule, pattern: e.target.value })
            }}
          />
        </label>
        {rule.kind === 'redact' ? (
          <label className="space-y-1 text-xs text-muted-foreground">
            {t('rules.replace')}
            <Input
              value={rule.replace ?? ''}
              className="font-mono"
              placeholder={t('rules.replacePlaceholder')}
              onChange={(e) => {
                onChange({ ...rule, replace: e.target.value })
              }}
            />
          </label>
        ) : null}
      </div>
      <div className="flex gap-1">
        <Button
          type="button"
          variant="ghost"
          size="icon"
          disabled={index === 0}
          aria-label={t('rules.moveUp', { index: n })}
          onClick={() => {
            onMove(-1)
          }}
        >
          <ArrowUpGlassIcon size="1.2em" />
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          disabled={index === count - 1}
          aria-label={t('rules.moveDown', { index: n })}
          onClick={() => {
            onMove(1)
          }}
        >
          <ArrowDownGlassIcon size="1.2em" />
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          aria-label={t('rules.remove', { index: n })}
          onClick={onRemove}
        >
          <TrashGlassIcon size="1.2em" />
        </Button>
      </div>
    </li>
  )
}
