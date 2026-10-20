import { Checkbox } from '@/components/ui/checkbox'
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldError,
  FieldLabel,
  FieldTitle,
} from '@/components/ui/field'
import { ABILITY_OPTIONS, toggleAbility } from '../types/token'
import type { Ability } from '../types/token'

// AbilitiesField is a plain controlled input (value/onChange), not a
// react-hook-form Controller itself: each caller wires its own Controller
// around this, the same way CreateTokenDialog originally did inline,
// keeping this component free of any one form library's generics. Shared
// by CreateTokenDialog, CreateUserDialog, and EditUserAbilitiesDialog:
// see types/token.ts's ABILITY_OPTIONS/toggleAbility doc comments for why
// the data itself lives there instead of here.
export function AbilitiesField({
  value,
  onChange,
  error,
}: {
  value: Ability[]
  onChange: (next: Ability[]) => void
  error?: { message?: string }
}) {
  return (
    <Field>
      <FieldLabel>Abilities</FieldLabel>
      <div className="space-y-2">
        {ABILITY_OPTIONS.map((option) => {
          const rootSelected = value.includes('root')
          const disabled = rootSelected && option.value !== 'root'
          return (
            // Boxed checkbox-card pattern: FieldLabel wrapping a Field is
            // what field.tsx's own classes are built for
            // (has-[>[data-slot=field]]:border,
            // has-data-checked:border-primary/30), so each ability gets a
            // bordered row that highlights on selection instead of a bare
            // checkbox + text line.
            <FieldLabel key={option.value}>
              <Field orientation="horizontal">
                <Checkbox
                  checked={value.includes(option.value)}
                  disabled={disabled}
                  onCheckedChange={() => {
                    onChange(toggleAbility(value, option.value))
                  }}
                />
                <FieldContent>
                  <FieldTitle>{option.label}</FieldTitle>
                  <FieldDescription className="text-xs">
                    {option.hint}
                  </FieldDescription>
                </FieldContent>
              </Field>
            </FieldLabel>
          )
        })}
      </div>
      <FieldError errors={[error]} />
    </Field>
  )
}
