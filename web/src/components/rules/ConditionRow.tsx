import { useTranslation } from 'react-i18next'
import { XGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { RuleSelect } from './RuleSelect'
import {
  changeField,
  changeOperator,
  fieldOf,
  operatorOf,
  type FieldDef,
  type RuleCondition,
  type RuleErrors,
} from './ruleModel'

export function ConditionRow({
  index,
  fields,
  condition,
  errors,
  removable,
  onChange,
  onRemove,
}: {
  index: number
  fields: FieldDef[]
  condition: RuleCondition
  errors: RuleErrors
  removable: boolean
  onChange: (c: RuleCondition) => void
  onRemove: () => void
}) {
  const { t } = useTranslation('rules')
  const field = fieldOf(fields, condition.field)
  const valueless = operatorOf(fields, condition.field, condition.op)?.valueless
  const at = `conditions[${index}]`
  const n = index + 1
  const nameError = errors[`${at}.name`]
  const valueError = errors[`${at}.value`] ?? errors[`${at}.op`]

  return (
    <div className="space-y-1">
      <div className="flex flex-wrap items-center gap-2 rounded-lg bg-muted/40 p-2">
        <span className="w-8 shrink-0 text-xs text-muted-foreground">
          {index === 0 ? t('builder.if') : t('builder.and')}
        </span>
        <RuleSelect
          aria-label={t('builder.fieldLabel', { n })}
          value={condition.field}
          options={fields.map((f) => ({ value: f.id, label: f.label }))}
          onChange={(e) =>
            onChange(changeField(fields, condition, e.target.value))
          }
        />
        {field?.named ? (
          <Input
            aria-label={t('builder.nameLabel', { n })}
            aria-invalid={nameError ? true : undefined}
            className="w-36"
            placeholder={field.namePlaceholder}
            value={condition.name}
            onChange={(e) => onChange({ ...condition, name: e.target.value })}
          />
        ) : null}
        <RuleSelect
          aria-label={t('builder.operatorLabel', { n })}
          value={condition.op}
          options={(field?.operators ?? []).map((o) => ({
            value: o.id,
            label: o.label,
          }))}
          onChange={(e) =>
            onChange(changeOperator(fields, condition, e.target.value))
          }
        />
        {valueless ? (
          <span className="flex-1" />
        ) : (
          <Input
            aria-label={t('builder.valueLabel', { n })}
            aria-invalid={valueError ? true : undefined}
            className="min-w-40 flex-1 font-mono"
            placeholder={field?.valuePlaceholder}
            value={condition.value}
            onChange={(e) => onChange({ ...condition, value: e.target.value })}
          />
        )}
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          disabled={!removable}
          aria-label={t('builder.removeCondition', { n })}
          onClick={onRemove}
        >
          <XGlassIcon size="1.2em" />
        </Button>
      </div>
      {nameError || valueError ? (
        <p role="alert" className="pl-10 text-xs text-destructive">
          {nameError ?? valueError}
        </p>
      ) : null}
    </div>
  )
}
