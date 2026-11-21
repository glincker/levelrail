import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Field, FieldError, FieldHint, FieldLabel } from '@/components/ui/field'
import { isFieldVisible, type FieldDescriptor } from '../types/fieldDescriptor'

interface DescriptorFieldsProps {
  fields: FieldDescriptor[]
  values: Record<string, string>
  onChange: (key: string, value: string) => void
  idPrefix: string
  /** Validation message per wire key, shown under the matching input. */
  errors?: Record<string, string | undefined>
  /**
   * Replaces the default rendering of one field (brand pickers, host key checks, reveal
   * toggles). Return undefined to keep the default, null to hide the field.
   */
  renderField?: (field: FieldDescriptor) => ReactNode | undefined
  /** Extra node beside a label, for example a brand tile. */
  labelAdornment?: (field: FieldDescriptor) => ReactNode
  /** Overrides the descriptor placeholder for one field. */
  placeholder?: (field: FieldDescriptor) => string | undefined
  /** Text appended to the label of optional fields. */
  optionalLabel?: string
}

// Renders every visible field of a descriptor set in registry order. Select fields have no
// default input: the caller supplies them through renderField because they carry brand tiles.
// Secrets use password inputs and are never prefilled.
export function DescriptorFields({
  fields,
  values,
  onChange,
  idPrefix,
  errors,
  renderField,
  labelAdornment,
  placeholder,
  optionalLabel,
}: DescriptorFieldsProps) {
  const { t: tKey } = useTranslation()
  // Keys come from the server registry, so they cannot be checked at compile time.
  const t = tKey as (key: string) => string
  return (
    <>
      {fields
        .filter((f) => isFieldVisible(f, values))
        .map((f) => {
          const custom = renderField?.(f)
          if (custom !== undefined) return custom
          if (f.type === 'select' || f.type === 'bool') return null
          const id = `${idPrefix}-${f.id.replace(/\./g, '-')}`
          const common = {
            id,
            value: values[f.key] ?? '',
            autoComplete: 'off',
            placeholder: placeholder?.(f) ?? f.placeholder,
            'aria-describedby': `${id}-help`,
            onChange: (
              e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>,
            ) => {
              onChange(f.key, e.target.value)
            },
          }
          const message = errors?.[f.key]
          return (
            <Field key={f.id}>
              <FieldLabel htmlFor={id}>
                <span className="flex items-center gap-2">
                  {t(f.label_key)}
                  {!f.required && optionalLabel ? (
                    <span className="text-muted-foreground">
                      {optionalLabel}
                    </span>
                  ) : null}
                  {labelAdornment?.(f)}
                </span>
              </FieldLabel>
              {f.type === 'text' ? (
                <Textarea
                  className="font-mono"
                  spellCheck={false}
                  {...common}
                />
              ) : (
                <Input
                  type={f.secret ? 'password' : 'text'}
                  className="font-mono"
                  {...common}
                />
              )}
              <FieldHint id={`${id}-help`} href={f.help_url}>
                {t(f.help_key)}
              </FieldHint>
              {message ? <FieldError>{message}</FieldError> : null}
            </Field>
          )
        })}
    </>
  )
}
