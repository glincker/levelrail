import { useState, type FormEvent, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { PlusGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { ActionRow } from './ActionRow'
import { ConditionRow } from './ConditionRow'
import { RuleCodeView } from './RuleCodeView'
import {
  blankCondition,
  removeAt,
  replaceAt,
  type ActionDef,
  type FieldDef,
  type RuleDraft,
  type RuleErrors,
} from './ruleModel'

export interface RuleBuilderProps {
  fields: FieldDef[]
  actions: ActionDef[]
  value: RuleDraft
  onChange: (next: RuleDraft) => void
  /** Client-side checks that mirror the server's; return error text keyed by path. */
  validate: (draft: RuleDraft) => RuleErrors
  /** Errors the server returned for the last submit, keyed the same way. */
  serverErrors?: RuleErrors
  /** The rule exactly as stored, for the read-only code view. */
  toCode: (draft: RuleDraft) => string
  onSubmit: () => void
  onCancel: () => void
  submitLabel: string
  pendingLabel: string
  submitting?: boolean
  /** A failure that is not tied to one field. */
  error?: ReactNode
  namePlaceholder?: string
  notePlaceholder?: string
  maxConditions?: number
  /** Extra caller fields between the description and the footer, such as a scope picker. */
  extra?: ReactNode
}

const DEFAULT_MAX_CONDITIONS = 8

/** RuleBuilder edits a list of AND conditions and one action; the caller supplies the field and action catalogs. */
export function RuleBuilder({
  fields,
  actions,
  value,
  onChange,
  validate,
  serverErrors,
  toCode,
  onSubmit,
  onCancel,
  submitLabel,
  pendingLabel,
  submitting = false,
  error,
  namePlaceholder,
  notePlaceholder,
  maxConditions = DEFAULT_MAX_CONDITIONS,
  extra,
}: RuleBuilderProps) {
  const { t } = useTranslation('rules')
  const [showCode, setShowCode] = useState(false)
  const [tried, setTried] = useState(false)
  const local = validate(value)
  const errors: RuleErrors = tried
    ? { ...serverErrors, ...local }
    : { ...serverErrors }

  function submit(e: FormEvent) {
    e.preventDefault()
    setTried(true)
    if (Object.keys(local).length === 0) onSubmit()
  }

  return (
    <form onSubmit={submit} className="space-y-(--glinr-page-gap)" noValidate>
      <Card>
        <CardHeader>
          <CardTitle>{t('builder.conditions')}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          {value.conditions.map((c, i) => (
            <ConditionRow
              key={i}
              index={i}
              fields={fields}
              condition={c}
              errors={errors}
              removable={value.conditions.length > 1}
              onChange={(next) =>
                onChange({
                  ...value,
                  conditions: replaceAt(value.conditions, i, next),
                })
              }
              onRemove={() =>
                onChange({
                  ...value,
                  conditions: removeAt(value.conditions, i),
                })
              }
            />
          ))}
          {errors.conditions ? (
            <p role="alert" className="text-xs text-destructive">
              {errors.conditions}
            </p>
          ) : null}
          <div className="flex items-center gap-2 pt-1">
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={value.conditions.length >= maxConditions}
              onClick={() =>
                onChange({
                  ...value,
                  conditions: [...value.conditions, blankCondition(fields)],
                })
              }
            >
              <PlusGlassIcon size="1.4em" />
              {t('builder.addCondition')}
            </Button>
            {value.conditions.length >= maxConditions ? (
              <span className="text-xs text-muted-foreground">
                {t('builder.maxConditions', { count: maxConditions })}
              </span>
            ) : null}
          </div>
          <ActionRow
            actions={actions}
            action={value.action}
            params={value.params}
            errors={errors}
            onAction={(id) => onChange({ ...value, action: id, params: {} })}
            onParam={(id, v) =>
              onChange({ ...value, params: { ...value.params, [id]: v } })
            }
          />
        </CardContent>
      </Card>

      <div className="grid gap-4 md:grid-cols-2">
        <label className="space-y-1.5 text-sm font-medium">
          {t('builder.ruleName')}
          <Input
            value={value.name}
            aria-invalid={errors.name ? true : undefined}
            placeholder={namePlaceholder}
            onChange={(e) => onChange({ ...value, name: e.target.value })}
          />
          {errors.name ? (
            <span
              role="alert"
              className="block text-xs font-normal text-destructive"
            >
              {errors.name}
            </span>
          ) : null}
        </label>
        <label className="space-y-1.5 text-sm font-medium">
          {t('builder.description')}
          <Input
            value={value.note}
            aria-invalid={errors.note ? true : undefined}
            placeholder={notePlaceholder}
            onChange={(e) => onChange({ ...value, note: e.target.value })}
          />
          {errors.note ? (
            <span
              role="alert"
              className="block text-xs font-normal text-destructive"
            >
              {errors.note}
            </span>
          ) : null}
        </label>
      </div>
      {extra}

      {error ? (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}
      {showCode ? <RuleCodeView code={toCode(value)} /> : null}

      <div className="flex flex-wrap items-center justify-end gap-2">
        <Button type="button" variant="outline" onClick={onCancel}>
          {t('builder.cancel')}
        </Button>
        <Button
          type="button"
          variant="outline"
          aria-expanded={showCode}
          onClick={() => setShowCode((s) => !s)}
        >
          {showCode ? t('builder.hideCode') : t('builder.viewCode')}
        </Button>
        <Button type="submit" disabled={submitting}>
          {submitting ? pendingLabel : submitLabel}
        </Button>
      </div>
    </form>
  )
}
