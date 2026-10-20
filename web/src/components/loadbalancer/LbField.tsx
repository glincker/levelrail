import type { ReactNode } from 'react'
import { InfoTip } from '@/components/kit'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import type { LbFormErrors, LbFormState } from '../../lib/loadBalancer'

export type LbPatch = (patch: Partial<LbFormState>) => void

export interface LbSectionProps {
  form: LbFormState
  errors: LbFormErrors
  onChange: LbPatch
}

interface FieldProps {
  id: string
  label: string
  info: ReactNode
  error?: string
  children: ReactNode
}

export function LbField({ id, label, info, error, children }: FieldProps) {
  return (
    <div className="space-y-1.5">
      <div className="flex items-center gap-1">
        <Label htmlFor={id}>{label}</Label>
        <InfoTip label={`About ${label}`}>{info}</InfoTip>
      </div>
      {children}
      {error ? (
        <p id={`${id}-error`} role="alert" className="text-xs text-tone-danger">
          {error}
        </p>
      ) : null}
    </div>
  )
}

interface TextProps {
  id: string
  label: string
  info: ReactNode
  value: string
  onChange: (value: string) => void
  error?: string
  placeholder?: string
  numeric?: boolean
}

export function LbTextField({
  id,
  label,
  info,
  value,
  onChange,
  error,
  placeholder,
  numeric,
}: TextProps) {
  return (
    <LbField id={id} label={label} info={info} error={error}>
      <Input
        id={id}
        value={value}
        inputMode={numeric ? 'numeric' : 'text'}
        placeholder={placeholder}
        aria-invalid={error ? true : undefined}
        aria-describedby={error ? `${id}-error` : undefined}
        onChange={(e) => onChange(e.target.value)}
      />
    </LbField>
  )
}

interface ToggleProps {
  id: string
  title: string
  info: ReactNode
  checked: boolean
  onCheckedChange: (checked: boolean) => void
  children?: ReactNode
  footer?: ReactNode
}

export function LbToggleBlock({
  id,
  title,
  info,
  checked,
  onCheckedChange,
  children,
  footer,
}: ToggleProps) {
  return (
    <section className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <h3
          id={`${id}-title`}
          className="flex items-center gap-1 text-sm font-medium"
        >
          {title}
          <InfoTip label={`About ${title}`}>{info}</InfoTip>
        </h3>
        <Switch
          checked={checked}
          onCheckedChange={onCheckedChange}
          aria-labelledby={`${id}-title`}
        />
      </div>
      {checked ? (
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {children}
        </div>
      ) : null}
      {checked ? footer : null}
    </section>
  )
}
