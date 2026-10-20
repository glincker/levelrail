import { useTranslation } from 'react-i18next'
import { Input } from '@/components/ui/input'
import { RuleSelect } from './RuleSelect'
import type { ActionDef, ParamDef, RuleErrors } from './ruleModel'

function ParamInput({
  def,
  value,
  error,
  onChange,
}: {
  def: ParamDef
  value: string
  error?: string
  onChange: (v: string) => void
}) {
  if (def.kind === 'select') {
    return (
      <RuleSelect
        aria-label={def.label}
        aria-invalid={error ? true : undefined}
        value={value}
        options={def.options ?? []}
        onChange={(e) => onChange(e.target.value)}
      />
    )
  }
  return (
    <span className="inline-flex items-center gap-1.5">
      <Input
        aria-label={def.label}
        aria-invalid={error ? true : undefined}
        className="w-24 tabular-nums"
        type={def.kind === 'number' ? 'number' : 'text'}
        min={def.min}
        max={def.max}
        placeholder={def.placeholder}
        value={value}
        onChange={(e) => onChange(e.target.value)}
      />
      {def.unit ? (
        <span className="text-xs text-muted-foreground">{def.unit}</span>
      ) : null}
    </span>
  )
}

/** ActionRow is the "Then" row: the action picker plus whatever inputs the chosen action declares. */
export function ActionRow({
  actions,
  action,
  params,
  errors,
  onAction,
  onParam,
}: {
  actions: ActionDef[]
  action: string
  params: Record<string, string>
  errors: RuleErrors
  onAction: (id: string) => void
  onParam: (id: string, value: string) => void
}) {
  const { t } = useTranslation('rules')
  const current = actions.find((a) => a.id === action)
  const err =
    errors.action ??
    Object.entries(errors).find(([k]) => k.startsWith('params.'))?.[1]
  return (
    <div className="space-y-1">
      <div className="flex flex-wrap items-center gap-2 rounded-lg bg-muted/40 p-2">
        <span className="w-8 shrink-0 text-xs text-muted-foreground">
          {t('builder.then')}
        </span>
        <RuleSelect
          aria-label={t('builder.actionLabel')}
          aria-invalid={errors.action ? true : undefined}
          value={action}
          placeholder={t('builder.selectAction')}
          options={actions.map((a) => ({ value: a.id, label: a.label }))}
          onChange={(e) => onAction(e.target.value)}
        />
        {current?.params?.map((p) => (
          <ParamInput
            key={p.id}
            def={p}
            value={params[p.id] ?? ''}
            error={errors[`params.${p.id}`]}
            onChange={(v) => onParam(p.id, v)}
          />
        ))}
      </div>
      {current?.hint ? (
        <p className="pl-10 text-xs text-muted-foreground">{current.hint}</p>
      ) : null}
      {err ? (
        <p role="alert" className="pl-10 text-xs text-destructive">
          {err}
        </p>
      ) : null}
    </div>
  )
}
