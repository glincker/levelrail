import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { NativeSelect } from '@/components/ui/native-select'
import { Label } from '@/components/ui/label'
import {
  readDocument,
  serializeDocument,
  valuesFromTree,
  YamlParseError,
  type FieldValue,
  type FormValues,
} from './documentYaml'
import { IacFieldControl } from './IacFieldControls'
import {
  DEFAULT_IAC_KINDS,
  schemaToFields,
  type IacField,
  type IacKindModel,
  type IacSchema,
} from './schemaToFields'
import { validateValues } from './validateValues'

interface DocState {
  kind: string
  values: FormValues
  extra: Record<string, unknown>
  error: { line: number } | { unknownKind: string } | null
  // Last yaml this state produced, so a parent echoing it back is not re-parsed.
  source: string
  // Last value prop seen, so a parent that ignores onChange does not reset the form.
  prop: string
}

function fromYaml(
  yaml: string,
  models: Record<string, IacKindModel>,
  fallback: string,
): DocState {
  const base: DocState = {
    kind: fallback,
    values: {},
    extra: {},
    error: null,
    source: yaml,
    prop: yaml,
  }
  if (!yaml.trim()) return base
  try {
    const doc = readDocument(yaml)
    if (!doc.kind) return base
    const model = models[doc.kind]
    if (!model) return { ...base, error: { unknownKind: doc.kind } }
    return { ...base, kind: doc.kind, ...valuesFromTree(model, doc.tree) }
  } catch (e) {
    return {
      ...base,
      error: { line: e instanceof YamlParseError ? e.line : 1 },
    }
  }
}

// Fields are visible when no ancestor group is hidden; groups only show with a visible child.
function section(fields: IacField[], advanced: boolean): IacField[] {
  const keep = fields.filter(
    (f) => f.type !== 'group' && f.advanced === advanced,
  )
  const groups = new Set(
    keep.flatMap((f) =>
      f.path.slice(0, -1).map((_, i) => f.path.slice(0, i + 2).join('.')),
    ),
  )
  return fields.filter(
    (f) => (f.type === 'group' && groups.has(f.id)) || keep.includes(f),
  )
}

function FieldGrid({
  fields,
  state,
  issues,
  disabled,
  onField,
}: Readonly<{
  fields: IacField[]
  state: DocState
  issues: ReturnType<typeof validateValues>
  disabled: boolean
  onField: (id: string, v: FieldValue) => void
}>) {
  return (
    <div className="grid grid-cols-1 gap-x-3 gap-y-2.5 md:grid-cols-2">
      {fields.map((f) =>
        f.type === 'group' ? (
          <h3
            key={f.id}
            className="mt-1 text-xs font-semibold text-muted-foreground md:col-span-2"
          >
            {f.label}
          </h3>
        ) : (
          <IacFieldControl
            key={f.id}
            field={f}
            value={state.values[f.id]}
            issue={issues[f.id] ?? null}
            disabled={disabled}
            onChange={(v) => onField(f.id, v)}
          />
        ),
      )}
    </div>
  )
}

export function IacSchemaForm({
  schema,
  value,
  onChange,
  kinds = DEFAULT_IAC_KINDS,
}: Readonly<{
  schema: IacSchema
  value: string
  onChange: (yaml: string) => void
  kinds?: readonly string[]
}>) {
  const { t } = useTranslation('iacForm')
  const models = useMemo(() => {
    const out: Record<string, IacKindModel> = {}
    for (const k of kinds) out[k] = schemaToFields(schema, k)
    return out
  }, [schema, kinds])
  const first = kinds[0] ?? 'App'
  const [state, setState] = useState<DocState>(() =>
    fromYaml(value, models, first),
  )

  let current = state
  if (value !== state.prop) {
    current =
      value === state.source
        ? { ...state, prop: value }
        : fromYaml(value, models, first)
    setState(current)
  }

  const model = models[current.kind] ?? models[first]
  const issues = useMemo(
    () =>
      model && !current.error ? validateValues(model, current.values) : {},
    [model, current.values, current.error],
  )
  if (!model) return null
  const locked = current.error !== null

  const commit = (
    kind: string,
    values: FormValues,
    extra: Record<string, unknown>,
  ) => {
    const m = models[kind]
    if (!m) return
    const yaml = serializeDocument(m, values, extra)
    setState({
      kind,
      values,
      extra,
      error: null,
      source: yaml,
      prop: current.prop,
    })
    onChange(yaml)
  }
  const onField = (id: string, v: FieldValue) =>
    commit(current.kind, { ...current.values, [id]: v }, current.extra)
  const onKind = (kind: string) => {
    // Only the name carries over; other fields belong to the old kind's shape.
    const name = current.values['metadata.name']
    commit(kind, name === undefined ? {} : { 'metadata.name': name }, {})
  }

  const basic = section(model.fields, false)
  const advanced = section(model.fields, true)
  const advancedCount = advanced.filter((f) => f.type !== 'group').length
  const errorText = current.error
    ? 'unknownKind' in current.error
      ? t('unknownKind', { kind: current.error.unknownKind })
      : t('parseError', { line: current.error.line })
    : null

  return (
    <form
      aria-label={t('formLabel', { kind: model.kind })}
      className="flex flex-col gap-3 rounded-lg border bg-card p-3"
      onSubmit={(e) => e.preventDefault()}
    >
      <div className="flex flex-wrap items-center gap-2">
        <Label htmlFor="iac-form-kind" className="gap-1.5">
          {t('kindLabel')}
        </Label>
        <NativeSelect
          id="iac-form-kind"
          value={model.kind}
          onChange={(e) => onKind(e.target.value)}
        >
          {kinds.map((k) => (
            <option key={k} value={k}>
              {k}
            </option>
          ))}
        </NativeSelect>
        {model.stability === 'beta' && (
          <Badge variant="info" title={t('betaHint')}>
            {t('beta')}
          </Badge>
        )}
      </div>
      {errorText && (
        <p role="alert" className="text-sm text-destructive">
          {errorText}
        </p>
      )}
      <FieldGrid
        fields={basic}
        state={current}
        issues={issues}
        disabled={locked}
        onField={onField}
      />
      {advancedCount > 0 && (
        <details className="group rounded-md border bg-muted/20 px-2.5 py-1.5">
          <summary className="cursor-pointer rounded text-sm font-medium outline-none focus-visible:ring-2 focus-visible:ring-(--glinr-accent-solid)">
            {t('advanced', { count: advancedCount })}
          </summary>
          <div className="pt-2.5 pb-1">
            <FieldGrid
              fields={advanced}
              state={current}
              issues={issues}
              disabled={locked}
              onField={onField}
            />
          </div>
        </details>
      )}
      {model.unsupported.length > 0 && (
        <p className="text-xs text-muted-foreground">
          {t('unsupported', { count: model.unsupported.length })}
        </p>
      )}
    </form>
  )
}
