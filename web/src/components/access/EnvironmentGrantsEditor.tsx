import { useTranslation } from 'react-i18next'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Field,
  FieldContent,
  FieldLabel,
  FieldTitle,
  FieldDescription,
} from '@/components/ui/field'
import type { EnvironmentChoice } from '../../queries/userAccess'

export function EnvironmentGrantsEditor({
  environments,
  loading,
  value,
  onChange,
}: {
  environments: EnvironmentChoice[]
  loading: boolean
  value: string[]
  onChange: (next: string[]) => void
}) {
  const { t } = useTranslation('access')

  function toggle(id: string) {
    onChange(
      value.includes(id) ? value.filter((v) => v !== id) : [...value, id],
    )
  }

  return (
    <div className="space-y-2" data-testid="environment-grants">
      <p className="text-sm font-medium text-foreground">
        {t('userRole.grantsTitle')}
      </p>
      <p className="text-xs text-muted-foreground">
        {t('userRole.grantsHint')}
      </p>
      {loading ? (
        <p className="text-xs text-muted-foreground">
          {t('userRole.grantsLoading')}
        </p>
      ) : null}
      {!loading && environments.length === 0 ? (
        <p className="text-xs text-muted-foreground">
          {t('userRole.grantsEmpty')}
        </p>
      ) : null}
      {environments.map((env) => (
        <FieldLabel key={env.id}>
          <Field orientation="horizontal">
            <Checkbox
              checked={value.includes(env.id)}
              onCheckedChange={() => {
                toggle(env.id)
              }}
            />
            <FieldContent>
              <FieldTitle>{env.name}</FieldTitle>
              {env.kind ? (
                <FieldDescription className="text-xs">
                  {env.kind}
                </FieldDescription>
              ) : null}
            </FieldContent>
          </Field>
        </FieldLabel>
      ))}
    </div>
  )
}
