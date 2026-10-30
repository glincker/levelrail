import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ArrowDownGlassIcon,
  ArrowUpGlassIcon,
  PlusGlassIcon,
  TrashGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/components/ui/toast'
import { useFormActions, type AppForm } from '../../queries/forms'
import {
  FIELD_TYPES,
  defaultFields,
  effectiveFields,
  type FormField,
  type FormFieldType,
  type FormSchema,
} from '../../lib/formSnippet'

const NATIVE_SELECT =
  'h-8 w-full rounded-lg border border-input bg-transparent px-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring'

function FieldRow({
  f,
  i,
  last,
  onChange,
  onMove,
  onRemove,
}: {
  f: FormField
  i: number
  last: boolean
  onChange: (patch: Partial<FormField>) => void
  onMove: (dir: -1 | 1) => void
  onRemove: () => void
}) {
  const { t } = useTranslation('forms')
  const id = `field-${i}`
  const textual =
    f.type !== 'checkbox' && f.type !== 'select' && f.type !== 'hidden'
  return (
    <li className="space-y-2 rounded-lg border border-border p-3">
      <div className="grid gap-2 sm:grid-cols-[1fr_1fr_9rem]">
        <div className="space-y-1">
          <Label htmlFor={`${id}-label`}>{t('designer.label')}</Label>
          <Input
            id={`${id}-label`}
            value={f.label ?? ''}
            onChange={(e) => onChange({ label: e.target.value })}
          />
        </div>
        <div className="space-y-1">
          <Label htmlFor={`${id}-name`}>{t('designer.name')}</Label>
          <Input
            id={`${id}-name`}
            value={f.name}
            onChange={(e) => onChange({ name: e.target.value.trim() })}
          />
        </div>
        <div className="space-y-1">
          <Label htmlFor={`${id}-type`}>{t('designer.type')}</Label>
          <select
            id={`${id}-type`}
            className={NATIVE_SELECT}
            value={f.type}
            onChange={(e) =>
              onChange({ type: e.target.value as FormFieldType })
            }
          >
            {FIELD_TYPES.map((ty) => (
              <option key={ty} value={ty}>
                {t(`designer.types.${ty}`)}
              </option>
            ))}
          </select>
        </div>
      </div>
      {textual && (
        <div className="grid gap-2 sm:grid-cols-3">
          <div className="space-y-1">
            <Label htmlFor={`${id}-ph`}>{t('designer.placeholder')}</Label>
            <Input
              id={`${id}-ph`}
              value={f.placeholder ?? ''}
              onChange={(e) => onChange({ placeholder: e.target.value })}
            />
          </div>
          <div className="space-y-1">
            <Label htmlFor={`${id}-min`}>{t('designer.minLength')}</Label>
            <Input
              id={`${id}-min`}
              type="number"
              min={0}
              value={f.min_length ?? ''}
              onChange={(e) =>
                onChange({ min_length: Number(e.target.value) || undefined })
              }
            />
          </div>
          <div className="space-y-1">
            <Label htmlFor={`${id}-max`}>{t('designer.maxLength')}</Label>
            <Input
              id={`${id}-max`}
              type="number"
              min={0}
              value={f.max_length ?? ''}
              onChange={(e) =>
                onChange({ max_length: Number(e.target.value) || undefined })
              }
            />
          </div>
        </div>
      )}
      {f.type === 'select' && (
        <div className="space-y-1">
          <Label htmlFor={`${id}-opts`}>{t('designer.options')}</Label>
          <Textarea
            id={`${id}-opts`}
            rows={3}
            value={(f.options ?? []).join('\n')}
            onChange={(e) => onChange({ options: e.target.value.split('\n') })}
          />
          <p className="text-xs text-muted-foreground">
            {t('designer.optionsHint')}
          </p>
        </div>
      )}
      {f.type === 'hidden' && (
        <div className="space-y-1">
          <Label htmlFor={`${id}-def`}>{t('designer.default')}</Label>
          <Input
            id={`${id}-def`}
            value={f.default ?? ''}
            onChange={(e) => onChange({ default: e.target.value })}
          />
        </div>
      )}
      <div className="flex items-center justify-between gap-2">
        {f.type === 'hidden' ? (
          <span />
        ) : (
          <Label className="font-normal">
            <Checkbox
              checked={f.required ?? false}
              onCheckedChange={(on) => onChange({ required: on })}
            />
            {t('designer.required')}
          </Label>
        )}
        <div className="flex gap-1">
          <Button
            variant="ghost"
            size="xs"
            disabled={i === 0}
            aria-label={t('designer.moveUp')}
            onClick={() => onMove(-1)}
          >
            <ArrowUpGlassIcon size="1.4em" />
          </Button>
          <Button
            variant="ghost"
            size="xs"
            disabled={last}
            aria-label={t('designer.moveDown')}
            onClick={() => onMove(1)}
          >
            <ArrowDownGlassIcon size="1.4em" />
          </Button>
          <Button
            variant="ghost"
            size="xs"
            aria-label={t('designer.remove')}
            onClick={onRemove}
          >
            <TrashGlassIcon size="1.4em" />
          </Button>
        </div>
      </div>
    </li>
  )
}

function Preview({
  fields,
  success,
}: {
  fields: FormField[]
  success: string
}) {
  const { t } = useTranslation('forms')
  return (
    <div className="space-y-3 rounded-lg border border-dashed border-border bg-muted/30 p-4">
      <p className="text-xs font-medium text-muted-foreground">
        {t('designer.preview')}
      </p>
      <form
        className="space-y-3"
        onSubmit={(e) => e.preventDefault()}
        aria-label={t('designer.preview')}
      >
        {fields.map((f, i) =>
          f.type === 'hidden' ? null : (
            <div key={i} className="space-y-1">
              <Label className={f.type === 'checkbox' ? 'font-normal' : ''}>
                {f.type === 'checkbox' && <Checkbox />}
                {f.label || f.name}
                {f.required && <span aria-hidden> *</span>}
              </Label>
              {f.type === 'textarea' && (
                <Textarea rows={3} placeholder={f.placeholder} />
              )}
              {f.type === 'select' && (
                <select className={NATIVE_SELECT}>
                  {(f.options ?? []).filter(Boolean).map((o) => (
                    <option key={o}>{o}</option>
                  ))}
                </select>
              )}
              {f.type !== 'textarea' &&
                f.type !== 'select' &&
                f.type !== 'checkbox' && (
                  <Input type={f.type} placeholder={f.placeholder} />
                )}
            </div>
          ),
        )}
        <Button type="submit" size="sm" variant="outline">
          {t('designer.send')}
        </Button>
      </form>
      <p className="text-xs text-muted-foreground">
        {t('designer.successLabel')}: {success || t('designer.successDefault')}
      </p>
    </div>
  )
}

export function FormDesigner({
  app,
  form,
  onDraft,
}: {
  app: string
  form: AppForm
  onDraft: (s: FormSchema) => void
}) {
  const { t } = useTranslation('forms')
  const { saveFields } = useFormActions(app)
  const [fields, setFields] = useState<FormField[]>(form.schema.fields)
  const [success, setSuccess] = useState(form.schema.success_message ?? '')
  const [origins, setOrigins] = useState(
    (form.schema.allowed_origins ?? []).join('\n'),
  )
  useEffect(() => {
    onDraft({ fields, success_message: success.trim() || undefined })
  }, [fields, success, onDraft])
  const patch = (i: number, p: Partial<FormField>) =>
    setFields((cur) => cur.map((f, j) => (j === i ? { ...f, ...p } : f)))
  const move = (i: number, dir: -1 | 1) =>
    setFields((cur) => {
      const a = cur[i]
      const b = cur[i + dir]
      if (!a || !b) return cur
      const next = [...cur]
      next[i] = b
      next[i + dir] = a
      return next
    })
  const add = () =>
    setFields((cur) => [
      ...(cur.length === 0 ? defaultFields() : cur),
      { name: `field${cur.length + 1}`, type: 'text', label: '' },
    ])

  const save = () =>
    saveFields.mutate(
      {
        form: form.name,
        schema: {
          fields: fields.map((f) => ({
            ...f,
            options:
              f.type === 'select'
                ? (f.options ?? []).map((o) => o.trim()).filter(Boolean)
                : undefined,
          })),
          success_message: success.trim() || undefined,
          allowed_origins: origins
            .split('\n')
            .map((o) => o.trim())
            .filter(Boolean),
        },
      },
      {
        onSuccess: () =>
          toast.add({ title: t('designer.saved'), type: 'success' }),
      },
    )

  const usingDefault = fields.length === 0
  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <div className="space-y-3">
        {usingDefault && (
          <p className="text-xs text-muted-foreground">
            {t('designer.usingDefault')}
          </p>
        )}
        <ul className="space-y-2">
          {fields.map((f, i) => (
            <FieldRow
              key={i}
              f={f}
              i={i}
              last={i === fields.length - 1}
              onChange={(p) => patch(i, p)}
              onMove={(d) => move(i, d)}
              onRemove={() => setFields((cur) => cur.filter((_, j) => j !== i))}
            />
          ))}
        </ul>
        <Button writeAction variant="outline" size="sm" onClick={add}>
          <PlusGlassIcon size="1.4em" />
          {usingDefault ? t('designer.customize') : t('designer.addField')}
        </Button>
        <div className="space-y-1.5">
          <Label htmlFor={`${form.id}-success`}>{t('designer.success')}</Label>
          <Input
            id={`${form.id}-success`}
            value={success}
            onChange={(e) => setSuccess(e.target.value)}
            placeholder={t('designer.successDefault')}
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor={`${form.id}-origins`}>{t('designer.origins')}</Label>
          <Textarea
            id={`${form.id}-origins`}
            rows={2}
            value={origins}
            onChange={(e) => setOrigins(e.target.value)}
            placeholder="https://example.com"
          />
          <p className="text-xs text-muted-foreground">
            {t('designer.originsHint')}
          </p>
        </div>
        {saveFields.isError && (
          <p role="alert" className="text-sm text-destructive">
            {saveFields.error.message}
          </p>
        )}
        <div className="flex gap-2">
          <Button writeAction disabled={saveFields.isPending} onClick={save}>
            {saveFields.isPending ? t('designer.saving') : t('designer.save')}
          </Button>
          {!usingDefault && (
            <Button variant="ghost" onClick={() => setFields([])}>
              {t('designer.reset')}
            </Button>
          )}
        </div>
      </div>
      <Preview fields={effectiveFields({ fields })} success={success} />
    </div>
  )
}
