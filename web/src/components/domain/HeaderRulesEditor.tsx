import { useTranslation } from 'react-i18next'
import {
  ArrowDownIcon,
  ArrowUpIcon,
  PlusIcon,
  TrashIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { FieldMessage, NativeSelect } from './PolicyShared'
import { moveItem } from './policyUtils'
import type { HeaderRule } from '../../queries/domainPolicyTypes'

// HeaderRulesEditor edits an ordered list; errors are keyed by the API's
// field paths, e.g. "rules[2].name".
export function HeaderRulesEditor({
  rules,
  onChange,
  errors,
  max,
}: {
  rules: HeaderRule[]
  onChange: (rules: HeaderRule[]) => void
  errors: Record<string, string>
  max: number
}) {
  const { t } = useTranslation('domainPolicies')
  const update = (i: number, patch: Partial<HeaderRule>) => {
    onChange(rules.map((r, j) => (j === i ? { ...r, ...patch } : r)))
  }
  return (
    <div className="space-y-2">
      {rules.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('headers.empty')}</p>
      ) : null}
      <ol className="space-y-2">
        {rules.map((r, i) => (
          <li
            key={i}
            className="space-y-1 rounded-md border border-border p-2"
            aria-label={t('headers.ruleLabel', { n: i + 1 })}
          >
            <div className="flex flex-wrap items-end gap-2">
              <NativeSelect
                id={`hr-side-${i}`}
                label={t('headers.side')}
                value={r.side}
                options={[
                  { value: 'request', label: t('headers.request') },
                  { value: 'response', label: t('headers.response') },
                ]}
                onChange={(v) => update(i, { side: v as HeaderRule['side'] })}
              />
              <NativeSelect
                id={`hr-op-${i}`}
                label={t('headers.op')}
                value={r.op}
                options={[
                  { value: 'set', label: t('headers.opSet') },
                  { value: 'add', label: t('headers.opAdd') },
                  { value: 'remove', label: t('headers.opRemove') },
                ]}
                onChange={(v) => {
                  const op = v as HeaderRule['op']
                  update(i, op === 'remove' ? { op, value: '' } : { op })
                }}
              />
              <label className="flex min-w-40 flex-1 flex-col gap-1 text-xs">
                <span className="text-muted-foreground">
                  {t('headers.name')}
                </span>
                <Input
                  value={r.name}
                  aria-invalid={Boolean(errors[`rules[${i}].name`])}
                  onChange={(e) => update(i, { name: e.target.value })}
                  placeholder="X-Frame-Options"
                />
              </label>
              {r.op !== 'remove' ? (
                <label className="flex min-w-40 flex-[2] flex-col gap-1 text-xs">
                  <span className="text-muted-foreground">
                    {t('headers.value')}
                  </span>
                  <Input
                    value={r.value ?? ''}
                    aria-invalid={Boolean(errors[`rules[${i}].value`])}
                    onChange={(e) => update(i, { value: e.target.value })}
                  />
                </label>
              ) : null}
              <div className="flex gap-1">
                <Button
                  type="button"
                  size="icon-sm"
                  variant="ghost"
                  aria-label={t('common.moveUp')}
                  disabled={i === 0}
                  onClick={() => onChange(moveItem(rules, i, i - 1))}
                >
                  <ArrowUpIcon />
                </Button>
                <Button
                  type="button"
                  size="icon-sm"
                  variant="ghost"
                  aria-label={t('common.moveDown')}
                  disabled={i === rules.length - 1}
                  onClick={() => onChange(moveItem(rules, i, i + 1))}
                >
                  <ArrowDownIcon />
                </Button>
                <Button
                  type="button"
                  size="icon-sm"
                  variant="ghost"
                  aria-label={t('common.remove')}
                  onClick={() => onChange(rules.filter((_, j) => j !== i))}
                >
                  <TrashIcon />
                </Button>
              </div>
            </div>
            <FieldMessage message={errors[`rules[${i}].side`]} />
            <FieldMessage message={errors[`rules[${i}].op`]} />
            <FieldMessage message={errors[`rules[${i}].name`]} />
            <FieldMessage message={errors[`rules[${i}].value`]} />
          </li>
        ))}
      </ol>
      <FieldMessage message={errors.rules} />
      <Button
        type="button"
        size="sm"
        variant="outline"
        disabled={rules.length >= max}
        onClick={() =>
          onChange([
            ...rules,
            { side: 'response', op: 'set', name: '', value: '' },
          ])
        }
      >
        <PlusIcon />
        {t('headers.add')}
      </Button>
    </div>
  )
}
