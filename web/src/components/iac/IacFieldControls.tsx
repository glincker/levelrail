import { useId } from 'react'
import { useTranslation } from 'react-i18next'
import { LockGlassIcon, PlusGlassIcon, TrashGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect } from '@/components/ui/native-select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import type { FieldValue, KvEntry } from './documentYaml'
import { isSecretLikeName, type IacField } from './schemaToFields'
import type { FieldIssue } from './validateValues'

interface Props {
  field: IacField
  value: FieldValue | undefined
  issue: FieldIssue | null
  disabled: boolean
  onChange: (next: FieldValue) => void
}

function KeyValueRows({ field, value, disabled, onChange }: Props) {
  const { t } = useTranslation('iacForm')
  const rows = Array.isArray(value) ? (value as KvEntry[]) : []
  const patch = (i: number, p: Partial<KvEntry>) =>
    onChange(rows.map((r, j) => (j === i ? { ...r, ...p } : r)))
  return (
    <div className="flex flex-col gap-1.5">
      {rows.map((row, i) => {
        const forced = Boolean(field.secretRefs) && isSecretLikeName(row.key)
        const secret = row.secret || forced
        return (
          <div
            key={i}
            className="grid grid-cols-[1fr_1fr_auto_auto] items-center gap-1.5"
          >
            <Input
              aria-label={t('keyPlaceholder')}
              placeholder={t('keyPlaceholder')}
              value={row.key}
              disabled={disabled}
              onChange={(e) => {
                const key = e.target.value
                const nowSecret =
                  Boolean(field.secretRefs) && isSecretLikeName(key)
                patch(i, { key, secret: row.secret || nowSecret })
              }}
            />
            <Input
              aria-label={
                secret ? t('secretRefPlaceholder') : t('valuePlaceholder')
              }
              placeholder={
                secret ? t('secretRefPlaceholder') : t('valuePlaceholder')
              }
              value={row.value}
              disabled={disabled}
              autoComplete="off"
              onChange={(e) => patch(i, { value: e.target.value })}
            />
            {field.secretRefs ? (
              <Switch
                size="sm"
                checked={secret}
                disabled={disabled || forced}
                aria-label={
                  row.key
                    ? t('secretToggle', { key: row.key })
                    : t('secretToggleNew')
                }
                onCheckedChange={(on) => patch(i, { secret: on })}
              />
            ) : (
              <span />
            )}
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              disabled={disabled}
              aria-label={
                row.key ? t('removeRow', { key: row.key }) : t('removeRowNew')
              }
              onClick={() => onChange(rows.filter((_, j) => j !== i))}
            >
              <TrashGlassIcon size="1.4em" aria-hidden="true" />
            </Button>
          </div>
        )
      })}
      <Button
        type="button"
        variant="outline"
        size="xs"
        className="self-start"
        disabled={disabled}
        aria-label={t('addRow', { label: field.label })}
        onClick={() =>
          onChange([...rows, { key: '', value: '', secret: false }])
        }
      >
        <PlusGlassIcon size="1.4em" aria-hidden="true" />
        {field.label}
      </Button>
    </div>
  )
}

// One labelled control per field; hints and issues are tied with aria-describedby.
export function IacFieldControl(props: Props) {
  const { field, value, issue, disabled, onChange } = props
  const { t } = useTranslation('iacForm')
  const uid = useId()
  const hintId = `${uid}-hint`
  const errId = `${uid}-err`
  const describedBy = [
    field.description || field.secret ? hintId : '',
    issue ? errId : '',
  ]
    .filter(Boolean)
    .join(' ')
  const common = {
    id: uid,
    disabled,
    'aria-invalid': issue ? true : undefined,
    'aria-describedby': describedBy || undefined,
  }
  const str = typeof value === 'string' ? value : ''
  const wide = field.type === 'keyValue' || field.type === 'stringList'
  const placeholder =
    field.default !== undefined
      ? t('defaultHint', { value: String(field.default) })
      : undefined

  let control: React.ReactNode
  if (field.type === 'keyValue') {
    control = <KeyValueRows {...props} />
  } else if (field.type === 'stringList') {
    control = (
      <Textarea
        {...common}
        rows={2}
        placeholder={t('listPlaceholder')}
        value={Array.isArray(value) ? (value as string[]).join('\n') : ''}
        onChange={(e) => onChange(e.target.value.split('\n'))}
      />
    )
  } else if (field.type === 'boolean') {
    control = (
      <Switch
        id={uid}
        disabled={disabled}
        aria-describedby={common['aria-describedby']}
        checked={typeof value === 'boolean' ? value : field.default === true}
        onCheckedChange={(on) => onChange(on)}
      />
    )
  } else if (field.type === 'enum') {
    control = (
      <NativeSelect
        {...common}
        value={str}
        onChange={(e) => onChange(e.target.value)}
      >
        <option value="">{placeholder ?? ''}</option>
        {field.options?.map((o) => (
          <option key={o} value={o}>
            {o}
          </option>
        ))}
      </NativeSelect>
    )
  } else if (field.secret) {
    control = (
      <Input
        {...common}
        type="text"
        autoComplete="off"
        placeholder={t('secretRefPlaceholder')}
        value={str}
        onChange={(e) => onChange(e.target.value)}
      />
    )
  } else {
    control = (
      <Input
        {...common}
        type={field.type === 'number' ? 'number' : 'text'}
        inputMode={field.type === 'number' ? 'numeric' : undefined}
        min={field.min}
        max={field.max}
        placeholder={placeholder}
        value={str}
        onChange={(e) => onChange(e.target.value)}
      />
    )
  }

  return (
    <div
      className={
        wide ? 'flex flex-col gap-1 md:col-span-2' : 'flex flex-col gap-1'
      }
    >
      <Label htmlFor={uid} className="gap-1">
        {field.secret && <LockGlassIcon size="1.2em" aria-hidden="true" />}
        {field.label}
        {field.effectiveRequired && (
          <span className="text-xs font-normal text-muted-foreground">
            {t('required')}
          </span>
        )}
      </Label>
      {control}
      {(field.description || field.secret) && (
        <p id={hintId} className="text-xs text-muted-foreground">
          {field.secret ? t('secretRefHint') : field.description}
        </p>
      )}
      {issue && (
        <p id={errId} role="alert" className="text-xs text-destructive">
          {t(`issues.${issue.code}`, { param: issue.param })}
        </p>
      )}
    </div>
  )
}
