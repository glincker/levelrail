import { useTranslation } from 'react-i18next'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Field, FieldLabel } from '@/components/ui/field'
import { isFieldVisible, type FieldDescriptor } from '../types/fieldDescriptor'

interface DescriptorFieldsProps {
  fields: FieldDescriptor[]
  values: Record<string, string>
  onChange: (key: string, value: string) => void
  idPrefix: string
}

// Renders the visible text-like fields of a descriptor set; select fields stay with the
// caller because they carry brand tiles. Secrets use password inputs and are never prefilled.
export function DescriptorFields({
  fields,
  values,
  onChange,
  idPrefix,
}: DescriptorFieldsProps) {
  const { t: tKey } = useTranslation()
  // Keys come from the server registry, so they cannot be checked at compile time.
  const t = tKey as (key: string) => string
  return (
    <>
      {fields
        .filter((f) => f.type !== 'select' && isFieldVisible(f, values))
        .map((f) => {
          const id = `${idPrefix}-${f.id.replace(/\./g, '-')}`
          const common = {
            id,
            value: values[f.key] ?? '',
            autoComplete: 'off',
            'aria-describedby': `${id}-help`,
            onChange: (
              e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>,
            ) => {
              onChange(f.key, e.target.value)
            },
          }
          return (
            <Field key={f.id}>
              <FieldLabel htmlFor={id}>{t(f.label_key)}</FieldLabel>
              {f.type === 'text' ? (
                <Textarea className="font-mono" {...common} />
              ) : (
                <Input
                  type={f.secret ? 'password' : 'text'}
                  className="font-mono"
                  {...common}
                />
              )}
              <p id={`${id}-help`} className="text-xs text-muted-foreground">
                {t(f.help_key)}
              </p>
            </Field>
          )
        })}
    </>
  )
}
