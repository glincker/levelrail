import type { ReactNode } from 'react'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import type { LbFormErrors, LbFormState } from '../lib/loadBalancer'

export type LbFormPatch = (patch: Partial<LbFormState>) => void

interface TextFieldProps {
  id: string
  label: string
  value: string
  onChange: (value: string) => void
  error?: string
  placeholder?: string
  description?: string
  inputMode?: 'numeric' | 'text'
}

export function LbTextField({
  id,
  label,
  value,
  onChange,
  error,
  placeholder,
  description,
  inputMode = 'text',
}: TextFieldProps) {
  return (
    <Field>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <Input
        id={id}
        value={value}
        inputMode={inputMode}
        placeholder={placeholder}
        aria-invalid={error ? true : undefined}
        onChange={(e) => onChange(e.target.value)}
      />
      {description ? <FieldDescription>{description}</FieldDescription> : null}
      <FieldError errors={error ? [{ message: error }] : []} />
    </Field>
  )
}

interface ToggleSectionProps {
  id: string
  title: string
  description: string
  checked: boolean
  onCheckedChange: (checked: boolean) => void
  children: ReactNode
}

export function LbToggleSection({
  id,
  title,
  description,
  checked,
  onCheckedChange,
  children,
}: ToggleSectionProps) {
  return (
    <section className="space-y-3 rounded-lg border p-4">
      <div className="flex items-start justify-between gap-4">
        <div className="space-y-0.5">
          <h3 id={`${id}-title`} className="text-sm font-medium">
            {title}
          </h3>
          <p className="text-xs text-muted-foreground">{description}</p>
        </div>
        <Switch
          checked={checked}
          onCheckedChange={onCheckedChange}
          aria-labelledby={`${id}-title`}
        />
      </div>
      {checked ? (
        <div className="grid gap-3 sm:grid-cols-2">{children}</div>
      ) : null}
    </section>
  )
}

export interface LbFieldsProps {
  form: LbFormState
  errors: LbFormErrors
  onChange: LbFormPatch
}
