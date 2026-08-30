import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { roleForAbilities } from '../queries/roles'
import type { RoleResource } from '../queries/roles'
import type { Ability } from '../types/token'

// RoleSelect is a convenience picker over AbilitiesField: choosing a
// named role sets the whole ability list in one action. Its selected
// value is always derived from the current abilities, never tracked as
// separate state, so editing a checkbox after picking a role correctly
// falls back to "Custom" instead of silently disagreeing with what's
// actually selected.
export function RoleSelect({
  roles,
  abilities,
  onChange,
}: {
  roles: RoleResource[]
  abilities: Ability[]
  onChange: (next: Ability[]) => void
}) {
  const matched = roleForAbilities(roles, abilities)
  const selected = matched?.name ?? 'custom'

  return (
    <Field>
      <FieldLabel htmlFor="role-select">Role</FieldLabel>
      <Select
        value={selected}
        onValueChange={(name) => {
          const role = roles.find((r) => r.name === name)
          if (role) {
            onChange(role.abilities)
          }
        }}
      >
        <SelectTrigger id="role-select" className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {roles.map((role) => (
            <SelectItem key={role.name} value={role.name}>
              {role.name.charAt(0).toUpperCase() + role.name.slice(1)}
            </SelectItem>
          ))}
          <SelectItem value="custom" disabled>
            Custom
          </SelectItem>
        </SelectContent>
      </Select>
      <FieldDescription className="text-xs">
        {matched?.description ??
          'A hand-picked ability set that does not match a curated role.'}
      </FieldDescription>
    </Field>
  )
}
